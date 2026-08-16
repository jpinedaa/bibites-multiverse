# Install

**Recommended setup:** Windows uses one setup executable. Linux uses a complete archive. Both
include an authorized copy of *The Bibites* and create a unique public-map identity. The Windows
GUI can use an existing game and can open the connected game after installation.

The Linux installer uses the included native game automatically. The Linux add-on remains
available if you already have the supported itch.io game. **What you do not need:** a join
string, compiler, SDK, administrator rights, or root.

`SHA256SUMS` sits beside all current packages:

| Platform | Download | Game source |
|---|---|---|
| Windows, recommended | `bibites-multiverse-0.2.5-windows-x64-setup.exe` | included portable game, or your existing Steam copy |
| Windows, advanced ZIP | `bibites-multiverse-0.2.5-windows-x64-complete.zip` | included portable game, or your existing Steam copy |
| Windows, add-on | `bibites-multiverse-0.2.5-windows-x64.zip` | your existing Steam copy |
| Linux, recommended | `bibites-multiverse-0.2.5-linux-x64-complete.zip` | included native game |
| Linux, add-on | `bibites-multiverse-0.2.5-linux-x64.zip` | your existing itch.io copy |

Every participant package includes `public-map.json`, the public join configuration. It contains
the deployed enrollment and relay addresses. It contains no world identity or secret. Each
installer creates a unique identity and secret. Only the secret stays private.

**The mod inside every package is the same file**, byte for byte: it is platform-independent IL.
What differs by platform is the sidecar, BepInEx flavour, and kit. A complete package adds
`game-payload.json`, `GAME-REDISTRIBUTION-NOTICE.txt`, and `game/`. No installer downloads the
game, mod, sidecar, or BepInEx while it runs. Each installer contacts the public HTTPS enrollment
endpoint only to create this installation's map identity.

**On Linux the installer needs five programs:** `sha256sum`, `awk`, `unzip`, `file`, and `curl`.
It checks them before it changes the game. BepInEx uses `file` to read the game architecture. The
installer uses `curl` for one HTTPS enrollment request. On Debian or Ubuntu, run
`sudo apt install unzip file curl` if one is absent. That is `INS-LINUXDEPS`, not a toolchain.

## Before you download

**The release page carries published checksums, and it carries them above the download link.**
This part is the same on both platforms and it is the one that matters most. Check the file you
got against the checksum on the page before you run anything:

```powershell
(Get-FileHash -Algorithm SHA256 .\bibites-multiverse-0.2.5-windows-x64-setup.exe).Hash -eq '<the value on the page>'
```

```sh
sha256sum bibites-multiverse-0.2.5-linux-x64-complete.zip
```

A match means you have the published file. If it does not match, delete the download and try
again; if it does not match twice, report it and do not run it. That is `INS-CHECKSUM`.

### On Windows: the mark of the web, and the execution policy

Windows marks a downloaded setup or archive. **Clear the mark only after its checksum passes:**

```powershell
Unblock-File .\bibites-multiverse-0.2.5-windows-x64-setup.exe
```

You can also right-click the setup, select **Properties**, and if present, select **Unblock**. The
checksum makes this a decision about one verified file. The setup then verifies every embedded
package file
against `MANIFEST.sha256`. That is `INS-MARKOFWEB`.

This community setup is not code-signed. Windows can show **Unknown publisher**. Continue only
after the setup SHA-256 matches the release page. `BibitesMultiverseLauncher.exe`, the program the
setup installs, is not signed either — but the installer clears the download mark from it by name,
after checking it against `MANIFEST.sha256`, so Windows should not ask a second time when you open
the **Bibites Multiverse** icon. If it does ask, or if an antivirus quarantines the launcher, that
is the same `INS-MARKOFWEB` situation one step later: the file came out of a setup whose checksum
you already matched.

Double-click the setup executable. The GUI uses the included portable game by default. Select
**Use a game that is already installed** to bind to another copy. The installer searches Steam
and common game locations. If it finds no game, the GUI says **Game not found** and keeps the
folder picker available.

The setup creates desktop and Start Menu icons named **Bibites Multiverse**. Both open
`BibitesMultiverseLauncher.exe`, the installed application. It also registers an
uninstaller in **Windows Settings → Apps → Installed apps**. All application files stay in your
user profile. Setup does not need administrator rights.

**Start The Bibites and connect after installation** is selected by default. Clear it if you want
to install without starting. The setup's command wrapper uses `RemoteSigned` for its own process
only. It does not change the policy for your account or computer. The package never uses
`ExecutionPolicy Bypass`.

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
of it travels inside **every** archive as `support-matrix.json` — the same bytes, so the words the
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
Windows row covers a four-day continuous run on the six-world test deployment. The Linux row is a
single 14-minute authenticated session on one machine, alongside a decompile of both builds that
puts every difference between them outside the types the mod patches.

`INS-GAMEBUILD` is the refusal, and it prints the matrix's own sentence: *"This release supports
one game build, and the game on this machine is not it… Nothing about the map can change this,
and there is no flag that skips it. Two ways forward: wait for a release whose matrix lists your
build, or put this machine on a build this matrix lists."* **Nothing is installed when it fires**
— the check runs before the mod framework, before the plugin, and before your credential is
written anywhere.

## What the installer does

**On Windows the installed application is `BibitesMultiverseLauncher.exe`**, and the **Bibites
Multiverse** desktop and Start Menu icons open it. It names the world it is set to, its sidecar
port, and whether the sidecar and the game are running, above a numbered menu:

```text
   1) Start this world            [Enter]
   2) Stop this world
   3) Status of every world
   4) Choose another world
   5) Edit this world's settings
   6) Create another world
   7) Delete a world
   8) Open this world's log folder
   0) Quit
```

An empty line selects **1**, so the icon and one key connect you. The same program also takes
commands, for a shortcut, a script, or a scheduled task:

```powershell
BibitesMultiverseLauncher.exe start                # the sidecar, then the game
BibitesMultiverseLauncher.exe start --headless     # no game window
BibitesMultiverseLauncher.exe start --no-headless  # draw it, even if this world is set headless
BibitesMultiverseLauncher.exe stop                 # the game, then the sidecar
BibitesMultiverseLauncher.exe stop --all           # every world on this computer
BibitesMultiverseLauncher.exe status --all
BibitesMultiverseLauncher.exe profile list
```

Advanced users can still run the generated scripts from the installed application directory. They
hold the values this world was installed with, and they start the same world:

```powershell
.\Start-Multiverse.ps1              # the sidecar, then the game
.\Stop-Multiverse.ps1               # the game, then the sidecar
```

**Use one or the other for a world, never both at once.** The scripts and the launcher share that
world's data folder and its `sidecar.pid` and `game.pid` files, but nothing coordinates the two:
run the script while the launcher is starting the same world and you can get two sidecars racing
for one port. The scripts also only know the values the world was installed with, so a setting you
changed in the launcher is not in them. A world the launcher created has no scripts at all.

The advanced ZIP also contains `Install-BibitesMultiverse.ps1`.

**Linux keeps its scripts as the entry point.** The launcher is a Windows program and is not part
of the Linux kit in this release:

```sh
./install-bibites-multiverse.sh     # install and enroll this Linux world
./start-multiverse.sh
./stop-multiverse.sh
```

All participant packages include the public-map connection details. Each installer reads
`public-map.json`, generates a different secret on this computer, and enrolls it over HTTPS. A
literal private-map join string is not packaged because each installation needs a different
identity and secret.

**Installing again over the same data root keeps that world.** An upgrade, a repair, a changed
game folder, a re-run after a stop: none of them is a new world, and none of them asks the map for
a second identity. The installer reads the identity that is already in the data root — from
`install-record.json`, from a pending enrollment record carrying that same secret, from the
launcher's `profiles\*.json`, from the previous `Start-Multiverse.ps1`, or from `data\peer-id` and
`data\relay-url` beside the journal — says *"reusing the map identity already in …"*, and **leaves
`peer-secret.txt` exactly as it is**. That works for a private map as well as the public one: an
adopted world keeps its own relay, and the installer says so rather than moving it.

**A secret is replaced in one case only, and it is the case that is meant to replace one.** If you
pass a join string that names the world already in that folder — a **slot handover**, which mints a
new secret for the same identity — the file is rewritten and the old one is kept beside it as
`peer-secret.txt.<utc>.old`, for you to delete once the world connects. The installer does that only
when a file it wrote itself proves which world the folder holds: the install record, or a pending
record carrying that very secret. `data\peer-id`, a launcher profile and an old start script are
ordinary text, and a claim in one of them is enough to *keep* a world and never enough to *destroy*
one. Everything else stops with `INS-ENROLL` and changes nothing: a join string for a **different**
identity, a claim nothing proves, a secret no file can name, a `-RelayUrl` that points an adopted
world at another map. Nothing recovers a secret once it is gone — the relay keeps a verifier and
cannot print it again — which is why the refusals are refusals.

**A join string that names a different identity takes `-ReplaceWorldIdentity` /
`--replace-world-identity`.** A **slot handover** mints exactly that — a new identity with a fresh
credential, rebound to your old slot and position — and it is the map's only credential recovery; a
borrowed or mistyped join string looks the same from here and would leave the world that owns this
journal dark. The switch says which it is. The old name is then kept in `data\peer-id.previous`,
because that string is the whole of the message an operator needs.

**If the secret is gone and the name is not**, which is what an uninstall from a release before this
one left behind, the installer stops as well and prints the world's identity. Ask that world's
operator for the handover above and install the join string it prints with the same switch, or use
the switch on its own to take a new identity with no place: a **second world on the map**, with the
old one dark until its operator releases it. Every refusal here is `INS-ENROLL` in
[`../error-taxonomy.md`](../error-taxonomy.md), with the files to look in and the ways on.

**No parameter takes a private-map join string on the command line**, on purpose: a value typed there is
in every process listing on your machine and in your shell history, and the wire itself has the
same rule. `-JoinStringFile .\join.txt` — `--join-string-file ./join.txt` on Linux, and the same
flag on the launcher's `profile create` — reads it from
a file if you would rather not type it, and then delete that file, which the installer tells you
to do and will not do for you.

The Windows GUI selects the package's included portable game when it exists. It also offers an
existing-game selection. The advanced script has `-RuntimeSelection bundled|external`; `auto`
uses the included game when present. Linux selects the mode from package contents.

**Both installers work in nine steps they name on your screen as they go**, and the steps are the
same nine: check the package against `MANIFEST.sha256`; select the existing or bundled game
runtime; check the build against the
matrix; install BepInEx if it is not already there; copy the plugin; enroll a unique public-map
identity or split a private-map join string, then store the secret in a file only you can read;
arrange trust for a certificate authority
**only** if you gave it one for a private map; state the settings this install ships with; and
write the start script, the stop script, the application files, and the uninstall record. The
Windows installer writes one more thing in that last step: `profiles\default.json`, the world
profile the launcher reads.

**Three of those nine differ, and only three:**

| Step | On Windows | On Linux |
|---|---|---|
| 1, after the checksum | clears the mark of the web from the files it verified, by name | makes executable the files it verified, by name |
| 2, selecting the game | the GUI defaults to the included game; the existing-game option reads Steam's registry, library index, and common paths | add-on searches the itch app root and usual places; complete installs its payload below the data root |
| 7, a private map's authority | imports it into **your own user store**, `Cert:\CurrentUser\Root`, and records the thumbprint so the uninstall can take it out | writes it to **no store at all**. The copy goes beside your data and the start script sets `SSL_CERT_FILE` at it, for that one process. Nothing under `/etc/ssl` or `/usr/local/share/ca-certificates` is touched and `update-ca-certificates` is never run |

**Neither needs administrator rights or root**, adds a service, a scheduled task, or a systemd
unit. Neither touches your worlds or their backups. The Windows setup adds per-user shortcuts and
one uninstall entry. The Windows GUI starts the sidecar and game when its final checkbox is
selected. It is selected by default.

**A running game blocks only the copy it is running from.** Both installers refuse to write into a
game folder while a game is running out of **that** folder — on Windows because the platform holds
the plugin file open while the game runs, on Linux because replacing a file the running game has
mapped into memory is a way to crash a world mid-save. **A copy of the game in another folder
blocks nothing**, on either platform.

**On Windows that holds even for a copy this account cannot inspect.** Windows does not always let
one account read another's process path — another user's session, or an elevated one — and
*"I cannot tell"* is not the same answer as *"it is running here"*. So when it cannot tell, the
installer asks the folder instead of guessing: it tries to open `The Bibites.exe` and the plugin in
the folder it is about to write to. **A file something is holding open stops the install and is
named in the refusal.** Files nothing is holding let it continue, with a line saying that is what
it found. A game genuinely running from that folder is still refused, whoever owns it. On Linux the
check reads `/proc`, so a game **another user** is running out of your game folder is a case it
cannot see, and it says so.

**The uninstall.** On Windows, use **Settings → Apps → Installed apps → Bibites Multiverse**.
The advanced command is `Uninstall-BibitesMultiverse.ps1`. On Linux, use
`./uninstall-bibites-multiverse.sh`. Use `-DryRun` or `--dry-run` first for the ledger. It reads the record the
installer wrote and removes only what is named in it, checking each file's hash before it goes,
and prints a line per path for what it removed and what it kept. Four things it deliberately
keeps: **a file somebody changed after the install** — a changed plugin is reported and left;
**BepInEx**, whole, if it was on your machine before; **your journal**, which is the record of
organisms other worlds handed you; and **your world's identity** — `peer-secret.txt`, and
`data\peer-id` and `data\relay-url` beside the journal — because the world it names still has its
place on the map and only that secret can claim it. Those last two are removed together, and only
when you pass `-RemoveWorldData` / `--remove-world-data`, which says on your screen that it is the
end of that world on the map.
Its two refusals are the game still running from that folder and this install's own sidecar still
running — both stop before removing anything, and both name the command that fixes them. **They use
the same rule as the install**: only a game running from *that* folder counts, and where a process
cannot be inspected the uninstaller asks the folder's own files whether anything is holding them
open. It applies that to this install's game folder, to each extra world's game folder, and to the
sidecar in its own kit directory.

**On Windows the uninstall covers every world you added.** It walks the `profiles\` directory,
refuses while any of those worlds still has a live game or sidecar — `BibitesMultiverseLauncher.exe
stop --all` is the command it names — and then removes each world's process-id files, the profile
files themselves, and the `profiles\` directory. It keeps each extra world's journal, logs and
credential unless you pass `-RemoveWorldData`, which takes all three. Your save files are never
touched, on any path.

For a complete edition, the game payload is in the record too. Uninstall removes each unchanged
payload file by hash. It reports and keeps a changed file, a user-added file, and every directory
either one needs.

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

**Windows waits now too, and that is a change in this release.** Earlier releases ended the game
outright on Windows, which skipped the game's own shutdown and therefore skipped save-on-quit. The
launcher's **Stop this world**, `BibitesMultiverseLauncher.exe stop`, and the generated
`Stop-Multiverse.ps1` all ask the game to close first and force it only after the wait — 30 seconds
for the game, 10 for the sidecar. **A headless world is the exception:** it has no window to close,
so Windows cannot ask it, and the stop forces it. A headless world can lose the simulation since
its last save when you stop it, so give one a shorter save interval if that matters to you. That
is `LOCAL-HEADLESSSTOP` in [`../error-taxonomy.md`](../error-taxonomy.md), which has the whole of
it — including the one way to stop a headless world without losing anything.

## More than one world on this computer

**On Windows the launcher does this, and a second kit is not needed.** Select **Create another
world** in its menu, or run:

```powershell
BibitesMultiverseLauncher.exe profile create winter --world Winter --sidecar-port 8788
```

Both flags are optional. Without them the launcher names the world after the profile and takes the
lowest free port from 8787 upward. It gives the new world its own data folder as well, so each
world keeps a separate credential, journal, log folder, and pair of recorded process IDs. The
worlds share one game folder, one copy of BepInEx, and one plugin, because each game process reads
its own settings from its own environment.

`profile create` also takes `--data-root`, `--game-dir`, `--world`, `--sidecar-port`,
`--headless` / `--no-headless`, `--export-edges`, `--exclude-species`, `--no-migration-exclusion`,
`--save-minutes`, `--save-keep` and `--save-on-quit`. For a private map it takes
`--join-string-file`, and `--relay-url` **only** when that file holds the identity half on its own
— a whole `multiverse-join/1` line already carries the relay address, and the launcher never takes
a relay address from the flag over one the join file carries. On the public map `--relay-url` is
refused outright, because the packaged `public-map.json` is the address.

`profile set NAME` changes every one of those **except `--data-root`, `--join-string-file` and
`--relay-url`**. **A world's data folder, its identity and its relay address are fixed once it
exists**, because changing any of them would point one world's identity at another world's
journal. If you want a world somewhere else on disk, create another world with `--data-root` and
delete the old one when you have what you wanted from it. `profile list`, `profile use NAME`,
`profile show NAME` and `profile delete NAME` do the rest.

**Five worlds per game folder is the ceiling, and the launcher enforces it.** The mod framework
hands out five log files per game folder and then gives up, and a sixth game that gets no
log file **never loads the mod** — it comes up connected to nothing, loudly. That is
`LOCAL-STARVATION` in [`../error-taxonomy.md`](../error-taxonomy.md). The launcher refuses to start
a sixth world from one game folder and names that code in the refusal. If you want more than five,
install a second copy of the game into its own folder and point a profile at it with `--game-dir`.

**Every extra world is another identity on the public map, and that has consequences.** The
launcher enrolls the new world over HTTPS exactly as the installer did, so it creates a new
credential, a new peer identity, and a new place on the map. The map applies a limit per network
address, so several worlds created quickly can be refused with `429`; the launcher prints the
`Retry-After` value the service returns, and waiting that long is the whole remedy. **Deleting a
profile is not leaving the map.** `profile delete` removes the world from this computer; the map
side is a separate act, described in [leave.md](leave.md).

**Deleting asks you to type the world's name.** With `--remove-world-data` — which also takes that
world's journal, logs and credential — it asks **even when you pass the global `--yes`**. A blanket
"answer yes to everything" must not be able to erase a world's data without naming it. `--yes` still
answers every other question, including a plain `profile delete`.

**All of your worlds save into one folder** — the game keeps its saves per user, not per game copy
(see *Where your worlds live* below). So each world needs a name no other world on this computer
uses; the launcher refuses a repeat. Their saves also queue behind each other. If you run several
worlds, **give each one a different save interval** so they do not all write at the same minute.

## ONE GAME INSTANCE PER GAME FOLDER, on Linux

**The Linux kit ships no Bibites Multiverse launcher in this release, and its rule is unchanged:
one game instance per game folder.** The failure is quieter than the Windows one. **Every Linux instance loads the mod and
works.** What breaks is the evidence: all of them open the *same* `BepInEx/LogOutput.log`, each
with its own file offset, each truncating it at launch, and the file ends as mostly NUL bytes with
one session's header where several should be. Nothing tells you. The world runs, the map is joined,
the saves land — and the first thing you reach for when something eventually goes wrong is already
gone.

**If you want a second world on a Linux machine, unpack a second copy of the game into its own
folder** and install into that. It costs disk and nothing else. `LOCAL-LOGSHRED` in
[`../error-taxonomy.md`](../error-taxonomy.md) has the shape of it and how to recognise a shredded
log.

A complete edition makes the separate game copy itself. For each concurrent Linux world, unpack a
separate kit and choose a distinct data root and sidecar port. That gives each world its own game
folder, BepInEx log, credential, journal, and recorded process IDs.

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

**A default Windows or Linux install connects to the public map automatically.** It generates a
unique secret locally and gets a unique identity over HTTPS. If an enrollment response is lost,
the installer keeps a protected pending record and retries the same identity. It does not spend a
second map identity. A private-map install connects after you supply a join-string file.

Once connected, the export default means that organisms leave your world on every side and arrive
from every side.

If you want a wall on one side, say so: `-ExportEdges E,N` — `--export-edges E,N` on Linux — at
install time. Afterwards, use `BibitesMultiverseLauncher.exe profile set NAME --export-edges E,N`
on Windows, or the same value in the start script on Linux. If you want your world off the map, do
not start it.

## The settings a fresh install ships with

Four of them are worth reading before you start, because each one spends something of yours.

**On Windows the launcher is where you edit them, per world.** Select **Edit this world's
settings** in its menu, or run `BibitesMultiverseLauncher.exe profile set NAME --save-minutes 15`.
The launcher keeps the values in `profiles\<world>.json` beside the application and passes them to
the game as the environment variables below, so each world on this computer can carry its own.

**If you move or reinstall the game, correct the world's game folder first** —
`BibitesMultiverseLauncher.exe profile set NAME --game-dir "<the new folder>"`. The launcher checks
the game folder on every write, so any other edit, and any start, is refused until that path is
right; the refusal names the folder it looked in.

**On Linux they live in one file you can read and edit: the start script**, `start-multiverse.sh`,
written beside the installer. Windows keeps `Start-Multiverse.ps1` in the installed application
directory for the same purpose. Each value is written out
explicitly, *including* the ones that match the mod's own default, so that a future change to a
default cannot silently move what your world does.

**The environment variable names are the mod's own and are identical on both platforms**; only the
way a start script spells an assignment differs
(`$env:NAME = 'value'` against `export NAME='value'`).

| Setting | Variable the game reads | In the profile | Ships as | What it costs you |
|---|---|---|---|---|
| Export edges | `MULTIVERSE_EXPORT_EDGES` (installer: `-ExportEdges` / `--export-edges`) | `exportEdges` | `E,N,W,S` | Your world is a full member of the map in every direction. This is the default and it is stated in three places on purpose |
| Migration exclusion | `MULTIVERSE_MIGRATION_EXCLUDE` (installer: `-ExcludeSpecies` / `--exclude-species`) | `excludeSpecies` | `Basic bibite` | It keeps founder stock off the lanes. **An empty value turns the policy off**, which floods a shared map with seed genomes and looks entirely normal in the census while it happens — so the installer and the launcher both refuse an empty value and make you say `-NoMigrationExclusion` / `--no-migration-exclusion` if you mean it, and print what it costs when you do |
| Save interval and retained saves | `MULTIVERSE_SAVE_MINUTES`, `MULTIVERSE_SAVE_KEEP` (installer: `-SaveMinutes`, `-SaveKeep` / `--save-minutes`, `--save-keep`) | `saveMinutes`, `saveKeep` | `10` and `6` | Six copies of your world on your disk that you did not budget for — measured at about 2.4–2.9 MB in total on this project's own worlds. The interval is also how often your world pauses to write itself out — see [diagnose.md](diagnose.md). With several worlds on one computer, give each a different interval |
| Save on quit | `MULTIVERSE_SAVE_ON_QUIT` (installer: `-SaveOnQuit on\|off` / `--save-on-quit on\|off`) | `saveOnQuit` | `true` | Your world is written out when the game closes, so stopping is not losing. Every stop path waits for that save rather than killing the game outright, except a headless world on Windows, which has no window to close |

The same names appear in the game's own settings file — `<game folder>\BepInEx\config\`
`dev.multiverse.bibites.cfg` on Windows, `<game folder>/BepInEx/config/dev.multiverse.bibites.cfg`
on Linux — as `ExportEdges`, `MigrationExcludeSpecies`, `SaveMinutes`, `SaveKeep` and
`SaveOnQuit`. **The environment wins**, because the mod reads it before that file. That is also why
several worlds can share one game folder on Windows: each game process gets its own environment.
Edit the profile on Windows or the start script on Linux; the config file is there for somebody
starting the game another way.

**`Start-Multiverse.ps1` keeps the values this world was installed with.** It is not updated when
you change a world's settings in the launcher, and the launcher says so when you do. On Windows the
launcher is what your world runs from; the script stays for scripted and advanced use.

Every default this release ships with — these four, and the two the operator side has — is
audited in [`../defaults-audit.md`](../defaults-audit.md), with what a bare install does and a
verdict.

## Two flags no document here will ever ask you to pass

The relay and the sidecar each have a flag that disables their authentication for a
single-machine rehearsal. **No installer, script, page or document this project ships may
instruct you to use either one.** If something does, that is the defect.

They are named differently from each other on purpose, so that no instruction can confuse the
two, and neither is a thing a participant needs. One belongs to the relay and is not even in the
package; the other belongs to the sidecar, is off, and **nothing that starts a sidecar here passes
it** — not the launcher, not `Start-Multiverse.ps1`, and not `start-multiverse.sh`. Both are named
and audited in
[`../defaults-audit.md`](../defaults-audit.md), so that you can recognise one if somebody ever
tells you to type it.

**The Linux kit adds nothing to this list.** It asks for no `sudo`, pipes no download into a
shell, and writes to no system trust store — a private map's authority is trusted through
`SSL_CERT_FILE` for one process, which is narrower than trusting it machine-wide, not wider.

## Next

[join.md](join.md) — how public enrollment and private-map join strings work, what happens on your
first claim, and what joining publishes about your world.
