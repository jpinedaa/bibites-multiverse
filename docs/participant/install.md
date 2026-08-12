# Install

**What you need:** The Bibites, on one of the two platforms this release supports — the **Steam
copy on Windows**, or the **native Linux build from itch.io**. **What you do not need:** a
compiler, an SDK, a runtime, administrator rights, root, or anything from a developer's toolchain.
If an installation step asks you for one of those, stop — that is a defect, and reporting it is
the right response.

**Where it comes from.** One archive per platform, from this project's GitHub release page, with
`SHA256SUMS` beside them:

| Platform | Archive | The kit inside it |
|---|---|---|
| Windows / Steam | `bibites-multiverse-m5.0-windows-x64.zip` | `Install-BibitesMultiverse.ps1`, PowerShell |
| Linux / itch.io | `bibites-multiverse-m5.0-linux-x64.zip` | `install-bibites-multiverse.sh`, bash |

**The mod inside the two archives is the same file**, byte for byte: it is platform-independent
IL, a patch against managed types the game's Mono build carries identically on both. What differs
is the sidecar (a native binary), the mod framework (BepInEx `win_x64` against `linux_x64`), and
the kit. Nothing else is a download, and no installer here fetches anything from the internet
while it runs.

**On Linux the installer needs four programs and checks for all four before it does anything:**
`sha256sum` and `awk`, which are on any machine that boots; `unzip`; and `file`, which **BepInEx's
own launcher** uses to check the game binary's architecture. Without that last one the install
would look complete and the game would never start, so it is checked first, where nothing has
happened yet — `sudo apt install unzip file` on Debian or Ubuntu. That is `INS-LINUXDEPS`, and it
is a dependency of the mod framework rather than a toolchain: no compiler, no SDK, no runtime.

## Before you download

**The release page carries published checksums, and it carries them above the download link.**
This part is the same on both platforms and it is the one that matters most. Check the file you
got against the checksum on the page before you run anything:

```powershell
(Get-FileHash -Algorithm SHA256 .\bibites-multiverse-m5.0-windows-x64.zip).Hash -eq '<the value on the page>'
```

```sh
sha256sum bibites-multiverse-m5.0-linux-x64.zip
```

A match means you have the published file. If it does not match, delete the download and try
again; if it does not match twice, report it and do not run it. That is `INS-CHECKSUM`.

### On Windows: the mark of the web, and the execution policy

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

### On Linux: there is no mark of the web, and no ritual replaces it

**Nothing arrives quarantined and no control has to be cleared**, so neither this page nor the
release page invents a step to make the two platforms look alike. Both of the sections above are
Windows-only and neither has a Linux counterpart.

What Linux has instead is a permission bit, and the installer's own first step handles it in the
order that makes it a decision rather than a habit: **it verifies every file in the folder against
`MANIFEST.sha256`, and only then makes executable exactly the files it just verified, by name, and
nothing else.** Checksum first, then the executable bit — the same sentence as *checksum first,
then the mark*, because it is the ordering this project cares about rather than the control.

If your shell says *permission denied* when you run the installer, the archive lost its mode bits
on the way to you. `bash install-bibites-multiverse.sh` runs it once, and its step 1 fixes the
rest.

## The support matrix, and why the installer may stop

Steam updates The Bibites on its own schedule; the itch.io download updates nothing by itself,
which is the same problem from the other side. This project publishes a **support matrix**: for
each game version *and platform*, the mod and sidecar build that go with it. **The installer
checks your game's build against that matrix and stops if there is no entry.**

That refusal is doing its job. There is no flag to bypass it, and joining the map with an
unsupported build is not something the software offers. Either wait for a release that lists
your build, or install the build the matrix names.

**The matrix is a per-machine question and never a statement about the map.** Whether your
build can join *this map* is a different test, answered by the relay against the wire version —
see [join.md](join.md).

**Where it is published, and how to look yourself up.** The table is
[`../support-matrix.md`](../support-matrix.md), beside every release, and a machine-readable copy
of it travels inside **both** archives as `support-matrix.json` — the same bytes, so the words the
installer refuses with are the words on the page. It matches on the **SHA-256 of
`BibitesAssembly.dll`** rather than on a version string, because a version string is a value a
build can reuse. You can check yourself before downloading anything — the matrix page gives the
one-line command for each platform.

**A row is keyed on a game version AND a platform**, and today the matrix has two: game
**0.6.3.1** on Windows/Steam, and game **0.6.3.1** on Linux/itch.io, both with mod `0.6.4` and
sidecar `m5.0`. The same version is two different files — they differ by 512 bytes, in the native
file-dialog shim — so **each installer looks up only its own platform's rows.** Running the wrong
archive on the right game is `INS-GAMEBUILD`, and the refusal prints the rows so you can see which
archive you wanted.

**The two rows are not equally proven, and the matrix page says so in the rows themselves.** The
Windows one is weeks of continuous running on this project's own deployment; the Linux one is a
single 14-minute authenticated session on one machine, alongside a decompile of both builds that
puts every difference between them outside the types the mod patches.

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

```sh
./install-bibites-multiverse.sh     # the same, and the same hidden prompt
./start-multiverse.sh
./stop-multiverse.sh
```

**No parameter takes your join string on the command line**, on purpose: a value typed there is
in every process listing on your machine and in your shell history, and the wire itself has the
same rule. `-JoinStringFile .\join.txt` — `--join-string-file ./join.txt` on Linux — reads it from
a file if you would rather not type it, and then delete that file, which the installer tells you
to do and will not do for you.

**Both installers work in nine steps they name on your screen as they go**, and the steps are the
same nine: check the package against `MANIFEST.sha256`; find the game; check the build against the
matrix; install BepInEx if it is not already there; copy the plugin; split your join string and
store the secret half in a file only you can read; arrange trust for a certificate authority
**only** if you gave it one for a private map; state the settings this install ships with; and
write the start script, the stop script and the record the uninstall reads.

**Three of those nine differ, and only three:**

| Step | On Windows | On Linux |
|---|---|---|
| 1, after the checksum | clears the mark of the web from the files it verified, by name | makes executable the files it verified, by name |
| 2, finding the game | reads Steam's registry keys and its library index, including a second drive | looks in the itch app's own install root and the usual places, then asks you. There is no registry and no library index to read |
| 7, a private map's authority | imports it into **your own user store**, `Cert:\CurrentUser\Root`, and records the thumbprint so the uninstall can take it out | writes it to **no store at all**. The copy goes beside your data and the start script sets `SSL_CERT_FILE` at it, for that one process. Nothing under `/etc/ssl` or `/usr/local/share/ca-certificates` is touched and `update-ca-certificates` is never run |

**Neither needs administrator rights or root**, adds a service, a scheduled task, a systemd unit, a
registry entry or a desktop file, starts anything by itself, or touches your worlds or their
backups.

**The uninstall.** `Uninstall-BibitesMultiverse.ps1` or `./uninstall-bibites-multiverse.sh`, with
`-DryRun` / `--dry-run` first if you want the ledger without the act. It reads the record the
installer wrote and removes only what is named in it, checking each file's hash before it goes,
and prints a line per path for what it removed and what it kept. Three things it deliberately
keeps: **a file somebody changed after the install** — a changed plugin is reported and left;
**BepInEx**, whole, if it was on your machine before; and **your journal**, which is the record of
organisms other worlds handed you, unless you pass `-RemoveWorldData` / `--remove-world-data`.
Its two refusals are the game still running from that folder and this install's own sidecar still
running — both stop before removing anything, and both name the command that fixes them.

On Linux the uninstall also takes back the four files BepInEx's archive lays beside the game
binary — `run_bepinex.sh`, `libdoorstop.so`, `.doorstop_version` and its own `changelog.txt` —
and only if this installer put them there. There is no `--keep-certificate`, because there is
nothing to keep: nothing was ever written to a store.

## Starting the game, and the one real difference

**On Linux the game is started through BepInEx's own launcher**, and `start-multiverse.sh` does
it for you:

```sh
cd "<game folder>" && ./run_bepinex.sh "./The Bibites.x86_64"
```

The mod framework hooks the game through `LD_PRELOAD` and a shim library rather than through a
DLL the loader picks up, so the launcher has to be the thing that runs. Your settings are ordinary
environment variables and the game inherits them through it — plain paths, nothing translated,
and **no `WSLENV`**: what a variable says is what the mod opens.

`stop-multiverse.sh` sends `SIGTERM`, **waits** up to twenty seconds, and only then kills. The
wait is the point: your world's save-on-quit runs during that shutdown, and a world killed
outright loses everything since its last save. A clean quit with its save has measured at about
two seconds.

## ONE GAME INSTANCE PER GAME FOLDER, on Linux

**Windows and Linux both punish a second instance run out of one game folder, and they punish it
in opposite ways.** On Windows the mod framework hands out a fixed number of log files and then
gives up, and an instance that gets no log file **never loads the mod** — it comes up connected to
nothing, loudly. That is `LOCAL-STARVATION`.

**On Linux every instance loads the mod and works.** What breaks is the evidence: all of them open
the *same* `BepInEx/LogOutput.log`, each with its own file offset, each truncating it at launch,
and the file ends as mostly NUL bytes with one session's header where several should be. Nothing
tells you. The world runs, the map is joined, the saves land — and the first thing you reach for
when something eventually goes wrong is already gone.

**If you want a second world on this machine, unpack a second copy of the game into its own
folder** and install into that. It costs disk and nothing else. `LOCAL-LOGSHRED` in
[`../error-taxonomy.md`](../error-taxonomy.md) has the shape of it and how to recognise a shredded
log.

## Where your worlds live

**Not in this install's directory, and not in the game folder.** They are the game's own files, in
the game's own place:

| Platform | Your worlds |
|---|---|
| Windows | `%USERPROFILE%\AppData\LocalLow\The Bibites\The Bibites\Savefiles` |
| Linux | `${XDG_CONFIG_HOME:-$HOME/.config}/unity3d/The Bibites/The Bibites/Savefiles` |

Nothing in this package writes outside its own directory there, and no uninstall goes near them.

**On Linux, `XDG_CONFIG_HOME` moves the whole tree** — saves, scenarios, templates and the game's
preferences — if it is set before the game starts. That is how a world is put on a separate disk,
and it is a thing you do to the game rather than to this mod: the mod asks Unity where the save
folder is and writes there.

## What a bare install does, stated once

**An unconfigured install exports on all four edges.** Nothing configured means the whole
perimeter, not silence — that is the shipped default and it is deliberate. The installer says
so in its own output, and this document says so here, so that neither is the only place a
reader could have learned it.

**An unconfigured install also connects to nothing.** Without a relay address and a credential
there is no map to join, so the export default is a question about *what your world means to
do* rather than a question about safety. When you join, it means: organisms leave your world on
every side, and arrive from every side.

If you want a wall on one side, say so: `-ExportEdges E,N` — `--export-edges E,N` on Linux — at
install time, or the same value in the start script afterwards. If you want your world off the map
entirely, do not join it.

## The settings a fresh install ships with

Four of them are worth reading before you start, because each one spends something of yours.

**All of them live in one file you can read and edit: the start script**, `Start-Multiverse.ps1`
or `start-multiverse.sh`, which the installer writes beside itself. Each is written out
explicitly, *including* the ones that match the mod's own default, so that a future change to a
default cannot silently move what your world does. **The environment variable names are the mod's
own and are identical on both platforms**; only the way the file spells an assignment differs
(`$env:NAME = 'value'` against `export NAME='value'`).

| Setting | In the start script | Ships as | What it costs you |
|---|---|---|---|
| Export edges | `MULTIVERSE_EXPORT_EDGES` (installer: `-ExportEdges` / `--export-edges`) | `E,N,W,S` | Your world is a full member of the map in every direction. This is the default and it is stated in three places on purpose |
| Migration exclusion | `MULTIVERSE_MIGRATION_EXCLUDE` (installer: `-ExcludeSpecies` / `--exclude-species`) | `Basic bibite` | It keeps founder stock off the lanes. **An empty value turns the policy off**, which floods a shared map with seed genomes and looks entirely normal in the census while it happens — so the installer refuses an empty value and makes you say `-NoMigrationExclusion` / `--no-migration-exclusion` if you mean it, and prints what it costs when you do |
| Save interval and retained saves | `MULTIVERSE_SAVE_MINUTES`, `MULTIVERSE_SAVE_KEEP` (installer: `-SaveMinutes`, `-SaveKeep` / `--save-minutes`, `--save-keep`) | `10` and `6` | Six copies of your world on your disk that you did not budget for — measured at about 2.4–2.9 MB in total on this project's own worlds. The interval is also how often your world pauses to write itself out — see [diagnose.md](diagnose.md) |
| Save on quit | `MULTIVERSE_SAVE_ON_QUIT` (installer: `-SaveOnQuit on\|off` / `--save-on-quit on\|off`) | `true` | Your world is written out when the game closes, so stopping is not losing. On Linux the stop script waits for that save rather than killing the game outright |

The same names appear in the game's own settings file — `<game folder>\BepInEx\config\`
`dev.multiverse.bibites.cfg` on Windows, `<game folder>/BepInEx/config/dev.multiverse.bibites.cfg`
on Linux — as `ExportEdges`, `MigrationExcludeSpecies`, `SaveMinutes`, `SaveKeep` and
`SaveOnQuit`. **The values in the start script win**, because the mod reads its environment before
that file. Edit the start script; the config file is there for somebody starting the game another
way.

Every default this release ships with — these four, and the two the operator side has — is
audited in [`../defaults-audit.md`](../defaults-audit.md), with what a bare install does and a
verdict.

## Two flags no document here will ever ask you to pass

The relay and the sidecar each have a flag that disables their authentication for a
single-machine rehearsal. **No installer, script, page or document this project ships may
instruct you to use either one.** If something does, that is the defect.

They are named differently from each other on purpose, so that no instruction can confuse the
two, and neither is a thing a participant needs. One belongs to the relay and is not even in the
package; the other belongs to the sidecar, is off, and **neither generated start script passes
it** — not `Start-Multiverse.ps1` and not `start-multiverse.sh`. Both are named and audited in
[`../defaults-audit.md`](../defaults-audit.md), so that you can recognise one if somebody ever
tells you to type it.

**The Linux kit adds nothing to this list.** It asks for no `sudo`, pipes no download into a
shell, and writes to no system trust store — a private map's authority is trusted through
`SSL_CERT_FILE` for one process, which is narrower than trusting it machine-wide, not wider.

## Next

[join.md](join.md) — what a join string is, what happens on your first claim, and what joining
publishes about your world.
