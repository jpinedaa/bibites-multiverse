# Bibites Multiverse — install kit (Windows)

**This folder joins one Bibites world to a shared map.** Organisms leave your world through its
edges and arrive in other people's; theirs arrive in yours. Your world stays on your machine and
stays yours.

**The recommended package includes an authorized portable copy of The Bibites.** The GUI selects
it by default. You can instead use a supported game already on this machine. This archive also
includes `public-map.json`, the public join configuration. It contains the deployed service
addresses and no secret. **You do not need:** a compiler, an SDK, or administrator rights.

**This is the Windows kit.** It is embedded in the setup executable and included in the advanced
ZIP packages. Linux has separate complete and add-on archives with a bash kit. The mod is the
same file in all packages. The sidecar, mod framework, and
scripts are platform-specific. Nothing here will ever ask you to turn a
security control off — no execution-policy bypass, no `--insecure` flag, no skipped certificate
check. If any part of this package asks you for one of those, that is a defect and reporting it
is the right response.

For a private map, you need **a join string**, which the operator hands you out of band. It looks
like this, on one line:

```
multiverse-join/1 wss://<relay-host>/contract-b/v4 <your-world>.<secret>
```

## Before you run anything

The recommended Windows download is one setup executable. Its release page carries the SHA-256.
Check that value before you run the setup. This folder also ships in the advanced ZIP. For that
ZIP, check the archive and clear its mark before extraction.

The installer checks itself again anyway. Its first step verifies every file in this folder
against `MANIFEST.sha256` and refuses to go on if one of them disagrees.

## Install

The single-file setup opens this GUI automatically. In the advanced ZIP, double-click
`Install-BibitesMultiverse.cmd`.

The setup's command wrapper uses the `RemoteSigned` policy for its own process only. It does not
change the policy for your account or computer. The GUI uses the included portable game by default. Its other option
searches for an existing game and lets you select a folder. The final start checkbox is on by
default. For advanced installation options, run:

```powershell
.\Install-BibitesMultiverse.ps1
```

The public-map setup reads the packaged public join configuration. It creates a unique credential
for this installation over HTTPS. It does not reuse one packaged join string. For a private map,
use a join-string file. **There is no parameter that takes the secret on the command line**, on
purpose. A command-line value appears in process listings and shell history. If you would rather
not type it, put the one-line join string in a file and pass `-JoinStringFile .\join.txt` — then
delete that file.

The GUI defaults to the included game. Its other option finds Steam's copy or accepts a selected
folder. The installer checks the game build against `support-matrix.json` and stops if there is no
entry. Then it installs BepInEx, the plugin, the credential, the start and stop scripts, and
`profiles\default.json` — the world profile the launcher reads.

**Stop your worlds before you install again.** Windows does not let anything replace a program
while it runs, and the launcher, the commands, the sidecar and the mod are all files this setup
replaces. So it asks first: if any of them is running out of the folders this run writes into, it
stops at its very first step, names each program with its process id and path, and **changes
nothing at all**. Close them — **Stop every world** in the launcher's window, or
`multiverse-launcher.exe stop --all`, then close the window itself and the game — and run the setup
again. **It never ends anything for you**, because a world ended rather than stopped loses
everything it has simulated since its last save.

**Installing again over the same data root keeps that world.** An upgrade or a repair is not a new
world: step 6 reads the identity already in `%LOCALAPPDATA%\BibitesMultiverse` — from
`install-record.json`, from a pending enrollment record carrying that same secret, from
`profiles\*.json`, from the previous `Start-Multiverse.ps1`, or from `data\peer-id` and
`data\relay-url` — says *"reusing the map identity already in …"*, and leaves `peer-secret.txt`
untouched. It asks the map for nothing and spends no second place on it, and an adopted world keeps
its own relay whether that is the public map or a private one.

**The last place it looks is the sidecar's own log.** Every line the sidecar writes carries
`peer=<identity>`, and its startup line carries `relay=` beside it, so `logs\sidecar.log` and the
rotated files beside it still say which world a folder is when nothing else does. Setup reads them
itself and prints the line it took the identity from. Two worlds in those logs means the folder has
run two: it lists them with where each was found and asks you to put the right one into
`data\peer-id` rather than choosing for you. The kit folders it searches are bounded and named —
this application directory, the folder the kit was unpacked in,
`%LOCALAPPDATA%\Programs\Bibites Multiverse`, the data root, and the data root's parent, because an
advanced ZIP is usually unpacked beside the data it writes.

**A secret no world ever used is an orphan, not a question.** If an earlier install got a
credential and stopped before the world ever ran — nothing inside `<data root>\data`, no `peer=` in
a sidecar log — that secret belongs to no world and holds no place on the map: setup renames it to
`peer-secret.txt.<utc>.orphan`, keeps it, and enrolls a new identity.

**A secret is replaced only when something vouches for the world it belongs to** — this data
root's own install record, or a pending record carrying that very secret — and the replaced one is
kept beside the new file as `peer-secret.txt.<utc>.old`. A claim that only an ordinary text file
makes is enough to keep a world and never enough to destroy one. Everything else stops with
`INS-ENROLL` and changes nothing: a `peer-secret.txt` no file on this machine can name, a file that
is not a credential, a `-RelayUrl` that would point an adopted world at another map. Every refusal
prints the files to look in and the ways on.

**A different identity over this world takes `-ReplaceWorldIdentity`.** A slot handover mints
exactly that — a new identity with a fresh credential, rebound to your old slot and position, which
is the map's only credential recovery — and a borrowed or mistyped join string looks the same from
here. The switch says which it is, and the old name is kept in `data\peer-id.previous`. **If the
secret is gone and the name is not** — what an uninstall from a release before this one left — setup
stops and prints the world's identity: ask its operator for that handover, or use the switch alone
to take a new identity with no place, which is a second world on the map and leaves the old one dark
until its operator releases it.

It imports nothing into any trust store. `-CaFile` exists for a private or LAN map whose relay
signs its own certificate, and only then.

**What a bare install does:** your world **exports on all four edges**. Nothing configured means
the whole perimeter, not silence. Every edge is a door that works both ways. The installer says
so on your screen while it runs, and `docs/participant/install.md` says so on the page.

## Start and stop

The setup creates **Bibites Multiverse** icons on the desktop and Start Menu. Both open
`BibitesMultiverseLauncher.exe`, the installed application's **window**.

**On the left it lists every world on this computer**, one line each, saying in plain words and in
colour what that world is doing: grey **Stopped**, amber **Starting...**, green
**On the map - speed x10**, red **NOT on the map**. The red one is the one to know about — a world
can be running with its place on the map held and nothing actually crossing, and that is the state
this window exists to make visible.

**On the right is the world you selected**, with one big button — **Start** when it is stopped,
**Stop** when it is running — the check box **Run without a game window (headless)**, and this
world's save name, port, speed, folder and identity on the map. While anything is happening a bar
spins and a line says what stage it has reached; when it is over, one green or red line stays on
screen beside that world. **Show details** opens the whole of what the launcher did, and it opens
**by itself** whenever something goes wrong — with the launcher's own sentence about it in the panel
as well.

The rest is in two menus. **World**: **Run a health check**, **Edit settings...**,
**Clone this world...**, **Delete this world...**, **Set as the default world**,
**Create a world...**, **Stop every world**. **Open**: the data folder, the logs folder, the game's
own log, and the commands window.

**Closing the window does not stop your worlds.** They keep running; the window says so, and its
title bar says how many are up.

The commands are a second program in the same folder, `multiverse-launcher.exe`, and it is the one
to name in a shortcut, a script or a scheduled task. It also has the numbered console menu, which
is what it opens when it is run with no arguments on a terminal:

```powershell
multiverse-launcher.exe start                # the sidecar, then the game
multiverse-launcher.exe start --headless     # nothing drawn; the simulation is unchanged
multiverse-launcher.exe start --no-headless  # draw it, even if this world is set headless
multiverse-launcher.exe stop                 # the game, then the sidecar
multiverse-launcher.exe stop --game-only     # keeps your place on the map
multiverse-launcher.exe stop --all           # every world on this computer
multiverse-launcher.exe status --all
```

**Why two files.** PowerShell and `cmd` do not wait for a program that opens a window, so a script
that ran `stop` and then `status` against one dual-purpose executable would report on a world that
was still shutting down. `BibitesMultiverseLauncher.exe` given a command line passes it to
`multiverse-launcher.exe` and waits, so an old shortcut still works; a script should name
`multiverse-launcher.exe` itself.

`stop` asks the game to close before it forces it, so save-on-quit runs. **A headless world has no
window to close**, so it is asked through its own mod instead — the `quit` verb on this world's
command file, which runs the same shutdown and saves — and a headless stop loses nothing. The one
world that cannot be asked is one that was already running before this release was installed: the
mod learns its command file's name at start. Start it again once. That is `LOCAL-HEADLESSSTOP` in
`error-taxonomy.md`.

The generated scripts remain available in the installed application directory. They hold the
values this world was installed with:

```powershell
.\Start-Multiverse.ps1      # the sidecar, then the game
.\Stop-Multiverse.ps1       # the game, then the sidecar
```

**Use one or the other for a world, never both at once.** They share that world's data folder and
its `sidecar.pid` and `game.pid` files, and nothing coordinates them — running the script while the
launcher is starting the same world can leave two sidecars racing for one port. The scripts also
carry only the install-time values, so a setting changed in the launcher is not in them. Worlds the
launcher created have no scripts.

`Start-Multiverse.ps1 -Headless` runs the world with nothing drawn; the simulation is unchanged.
`Stop-Multiverse.ps1 -GameOnly` puts the world down and leaves the sidecar up, so it keeps your
place on the map and keeps taking custody of everything that arrives while you are away.

**Stopping costs nothing and needs nobody.** Your place on the map is keyed on your world's
identity and never expires.

## More than one world on this computer

**The launcher creates them, and a second kit is not needed.** Press **Create a world...**
in its window, or:

```powershell
multiverse-launcher.exe profile create winter --world Winter --sidecar-port 8788
multiverse-launcher.exe profile list
multiverse-launcher.exe profile use winter
multiverse-launcher.exe profile set winter --save-minutes 15
multiverse-launcher.exe profile delete winter
```

`profile delete` asks you to type the world's name. With `--remove-world-data`, which also takes
that world's journal, logs and credential, it asks **even under the global `--yes`** — a blanket
yes must not be able to erase a world's data without naming it.

Both creation flags are optional: without them the launcher names the world after the profile and
takes the lowest free port from 8787 upward. Each world gets its own data folder, credential,
journal, log folder, and process ledger, and they share one game folder, one copy of BepInEx, and
one plugin.

`profile create` also takes `--data-root`, `--game-dir`, `--world`, `--sidecar-port`,
`--headless` / `--no-headless`, `--export-edges`, `--exclude-species`, `--no-migration-exclusion`,
`--save-minutes`, `--save-keep` and `--save-on-quit`; for a private map, `--join-string-file`, and
`--relay-url` **only** when that file holds the identity half on its own. `profile set NAME`
changes every one of those **except `--data-root`, `--join-string-file` and `--relay-url`**: a
world's data folder, its identity and its relay address are fixed once it exists, because changing
any of them would point one world's identity at another world's journal. For a world in a
different folder, create another one with `--data-root`.

**Five worlds per game folder is the ceiling**, because the mod framework hands out five log files
per game folder and a game that gets no log file never loads the mod. The launcher refuses to start
a sixth and names `LOCAL-STARVATION`. For more than five, install a second copy of the game and
point a profile at it with `--game-dir`.

**Each extra world enrolls its own identity on the public map.** The map limits enrollments per
network address, so several worlds created quickly can be refused; the launcher prints the
`Retry-After` value the service returns. **Deleting a profile is not leaving the map** — the slot
stays reserved for that identity until the operator releases it. The published `leave` page has
the whole of it.

Every world on this machine saves into one folder, so each needs a world name no other world uses,
and their saves queue behind each other. Give each world a different `--save-minutes`.

## The settings this install ships with

All five are written explicitly, including the ones that match the mod's own default, so that a
future change to a default cannot silently move what your world does. **The launcher edits them
per world** — **Edit settings...** in its window, or
`multiverse-launcher.exe profile set NAME` — and keeps them in `profiles\<world>.json`. It
passes them to the game as the environment variables below.

`Start-Multiverse.ps1` holds the values this world was installed with and is not updated when you
change them in the launcher. On Windows the launcher is what your world runs from.

**If you move or reinstall the game, correct the world's game folder first** —
`multiverse-launcher.exe profile set NAME --game-dir "<the new folder>"`. The game folder is
checked on every write, so any other edit, and any start, is refused until that path is right.

| Variable the game reads | In the profile | Ships as | What it spends |
|---|---|---|---|
| `MULTIVERSE_EXPORT_EDGES` | `exportEdges` | `E,N,W,S` | Your world is a full member of the map in every direction |
| `MULTIVERSE_MIGRATION_EXCLUDE` | `excludeSpecies` | `Basic bibite` | Keeps the game's starter species off a shared map's lanes. An empty value turns the policy off, and the installer and the launcher both make you say `-NoMigrationExclusion` / `--no-migration-exclusion` to mean it |
| `MULTIVERSE_SAVE_MINUTES` | `saveMinutes` | `10` | How often your world pauses to write itself out |
| `MULTIVERSE_SAVE_KEEP` | `saveKeep` | `6` | Six copies of your world on your disk |
| `MULTIVERSE_SAVE_ON_QUIT` | `saveOnQuit` | `true` | Your world is written out when the game closes |
| `MULTIVERSE_STARTUP_TIME_SCALE` | not in the profile | `10` | Your world starts at x10 rather than the game's own x1. It is a target: the game holds the speed down to keep your frame rate up, so a slower machine runs slower and stays smooth. The speed slider in the game moves it for a session; `off` here means the game's own x1 |

## Uninstall

For a setup installation, open **Windows Settings → Apps → Installed apps** and remove
**Bibites Multiverse**. The Start Menu also contains an uninstall shortcut.

```powershell
.\Uninstall-BibitesMultiverse.ps1 -DryRun   # the ledger, changing nothing
.\Uninstall-BibitesMultiverse.ps1
```

It reads the record the installer wrote and removes **only** what is named in it, hash-checked,
printing a line per path for what it removed and what it kept. Your worlds and their backups are
never touched. Your journal — the organisms this machine is holding for other worlds — **and your
world's identity**, `peer-secret.txt` beside it, are kept unless you pass `-RemoveWorldData`. That
is why installing again later comes back as the same world on the same slot, and why
`-RemoveWorldData` says on your screen that it is the end of that world on the map. In a complete
install, unchanged game-payload files are also removed; a changed or user-added file is reported
and kept.

**It covers every world you added.** It refuses while any of them is still running — stop them with
`multiverse-launcher.exe stop --all` — and then removes each world's process-id files, the
profile files, and the `profiles\` directory. Each extra world's journal, logs and credential are
kept on the same rule.

**Only a game running from the folder in question blocks it.** Another copy of the game elsewhere
does not, and neither does one running under an account this one cannot inspect: where Windows will
not say where a process started from, the check asks the folder's own files whether anything is
holding them open, and names the file when something is. The install does the same before it writes
to a game folder.

## If something goes wrong

Four pages are published with this release, and they are written for you rather than for whoever
built it:

- **install** — what the installer does, and what a bare install does.
- **join** — what a join string is, what happens on your first claim, and what joining publishes
  about your world.
- **diagnose** — what to read, in what order, and what to send when you ask for help.
- **leave** — what stopping, leaving and handing your place over actually mean.

`error-taxonomy.md` beside them lists every refusal this system can hand you, with its remedy
**and who has to act on it** — because the most likely failure on a shared map is one where the
machine that suffers is not the machine at fault.

**Never send anybody your join string**, in a screenshot, an issue or a log. No log in this
system prints one, and no diagnostic asks for one.

## What is in this folder

| File | What it is |
|---|---|
| `Install-BibitesMultiverse.cmd` | The advanced ZIP entry point |
| `Install-BibitesMultiverse-Gui.ps1` | The game selection and start-after-install window |
| `Find-BibitesGame.ps1` | The existing-game search shared by the GUI and installer |
| `Install-BibitesMultiverse.ps1` | The installer and its advanced options |
| `public-map.json` | The public join configuration: HTTPS enrollment and WSS relay addresses; no secret |
| `Uninstall-BibitesMultiverse.ps1` | The uninstaller |
| `bibites-multiverse.ico` | The application and setup icon |
| `BibitesMultiverse.dll` | The mod, a BepInEx plugin |
| `multiverse-sidecar.exe` | The program that speaks to the map on your world's behalf |
| `BibitesMultiverseLauncher.exe` | The installed application's window: it lists your worlds and what each of them is doing, starts and stops them, and manages more than one of them. The icons open this |
| `multiverse-launcher.exe` | The same launcher's commands and console menu. This is the one a script calls |
| `profiles\` | Created by the installer beside these files: one JSON file per world, plus `active.txt`. **Never contains a secret** |
| `BepInEx_win_x64_5.4.23.3.zip` | The mod framework, exactly as its own project publishes it |
| `support-matrix.json` | The game builds this release supports, and the words it refuses with. The setup installs a copy beside `multiverse-sidecar.exe`, which reads it whenever you run the diagnostic |
| `LICENSE`, `THIRD_PARTY_NOTICES.md` | The project's Apache-2.0 license and bundled dependency notices |
| `game-payload.json`, `GAME-REDISTRIBUTION-NOTICE.txt`, `game\` | Files that occur only in a complete package |
| `MANIFEST.sha256` | The SHA-256 of every file above, which the installer checks first |
