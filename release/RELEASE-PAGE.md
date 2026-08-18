# Bibites Multiverse `@@RELEASE@@`

Connect your Bibites world to a shared map. Organisms can move between the worlds on the map.
Your game and saves stay on your computer.

## What is new in `@@RELEASE@@`

**The launcher is a window.** The desktop and Start Menu icons open a list of every world on this
computer, kept up to date while they run: what is running, whether the game's mod has really
reached the map, the speed the world is achieving, and its place on the map. One big button
starts or stops the world you selected. Buttons beside it create, clone, delete and check a world,
and a details pane — closed until something goes wrong, when it opens by itself — carries the
launcher's own output. Closing the window leaves your worlds running, which the window says.

**A world starts at ten times the game's own speed.** It is a target and not a demand: the game
holds the speed down to keep your frame rate up, so a slower computer runs slower and stays
smooth. The speed slider in the game still moves it.

**Stopping a world no longer costs you anything, with or without a game window.** A stop now asks
the world to save and quit through the mod, which is the same shutdown that closing the game
window performs. A world running with no window used to be forced, and everything it had simulated
since its last save was lost.

**The setup finishes, and it refuses before it changes anything.** With **Start after installation**
selected, the setup used to wait for the world it had just started instead of for its own
installer, so it never wrote the shortcuts or the *Installed apps* entry. Both now appear. If
anything of this installation is running when you start setup, it stops at the very first step,
lists the programs to close, and changes nothing at all. And the *Installed apps* size figure is
measured from the program folder rather than by walking every file your world has ever written,
which on a long-running world took nearly half an hour with the setup already invisible.

**Installing again after an uninstall works.** An uninstall that followed a second install over
the same included game used to leave the mod framework behind in that copy of the game and remove
the game from around it, and the setup after that found a game folder with no game in it and
refused to overwrite it. Setup now says what it found in such a folder, removes it whole and
unpacks the game again, so it repairs a computer the earlier one stranded. The uninstall takes
that copy back whole in the first place — framework, log and cache with it — and the launcher
profile describing an installation that no longer exists goes too, which is what lets *Installed
apps* remove the entry and its folder. Nothing of yours is in there: your worlds are the game's
own saves, and your world's journal, logs and credential are beside that folder rather than inside
it. On Windows the uninstall also keeps its ledger as a file,
`<data root>\logs\uninstall-<utc>.log`, because an uninstall started from *Installed apps* leaves
no window to read afterwards.

**A world's disk use stays bounded on Windows.** The sidecar rewrites its own journal on a timer
to drop what is no longer needed. On Windows that rewrite had never once completed, because it
renamed its replacement over a file the same program still held open. Two worlds under test had
reached 718 MB and 132 MB of journal before it was found; the same worlds afterwards compacted ten
times in one session, the largest reclaiming 98 MB.

**An organism crosses once, and a crossing that is lost is counted.** Nothing is re-sent, nothing
is held back and brought home later. This is the participant half of the change the hosted map
already made, and it is why the network protocol reads `contract-b/4.1`. A world on the older
`contract-b/4.0` still joins.

**The health check asks about your own world's map.** *Run a health check* now passes that world's
own relay address and credential, so the two checks about your place on the map answer about the
map you are on rather than about a local address you never used. A healthy world on the public map
reports sixteen passes. The support matrix is installed beside the sidecar as well, so the check
can name the game build this computer is on.

**And the commands have their own file.** `multiverse-launcher.exe` is the same launcher's commands
and console menu, installed beside the window. Windows does not let a script wait for a window, so
a script or a scheduled task must call that file.

## Upgrading from an earlier release

- **Stop every world before you run the setup.** Use *Stop every world* in the launcher's window,
  or `multiverse-launcher.exe stop --all`. Setup refuses while anything of this installation is
  running, and it refuses before it has changed anything.
- **Install over the top.** Your world keeps its identity, its place on the map, its saves and its
  journal.
- **If an earlier uninstall left setup refusing to install again**, this one repairs it. It removes
  the game folder its predecessor emptied, unpacks the game there afresh, and says so while it
  does — there is nothing to delete by hand.
- **The icons do not change.** They open `BibitesMultiverseLauncher.exe` exactly as before; that
  file is now the window.
- **Scripts should call `multiverse-launcher.exe`**, which is installed beside it. An old script
  that calls `BibitesMultiverseLauncher.exe` with a command line still works — the window hands the
  command line over and waits for it — but a shell cannot wait for a window, so a script that
  chains two commands should name the console program.
- **The window remembers where it was** in `%APPDATA%\Bibites Multiverse\launcher-window.json`:
  its size, its position, and where you left the divider. Delete that file to start over.
- **The first start after upgrading compacts the journal**, which on a long-running world can free
  a great deal of disk at once.
- **A world that was already running before you upgraded cannot be asked to quit through its mod**,
  because the mod reads that setting once when the game starts. Start that world once with this
  release and every stop after it is lossless.

## What you need

- Windows: nothing else. The recommended setup includes The Bibites `0.6.3.1`.
- Linux: nothing else. The recommended package includes the native game.

Every participant package includes `public-map.json`, the public join configuration. It contains
the deployed HTTPS enrollment and WSS relay addresses. During installation, each package creates
a unique world identity and secret. It does not reuse one shared credential.

A private join string contains the map address, world identity, and secret. Do not post one in an
issue, screenshot, or log.

@@EDITION_NOTE@@

## Downloads

| Platform | File | SHA-256 |
|---|---|---|
@@COMPLETE_DOWNLOAD_ROWS@@| Windows add-on | [`@@ZIP_NAME@@`](https://github.com/@@REPO@@/releases/download/@@TAG@@/@@ZIP_NAME@@) | `@@ZIP_SHA256@@` |
| Linux add-on | [`@@LINUX_ZIP_NAME@@`](https://github.com/@@REPO@@/releases/download/@@TAG@@/@@LINUX_ZIP_NAME@@) | `@@LINUX_ZIP_SHA256@@` |
| All | [`SHA256SUMS`](https://github.com/@@REPO@@/releases/download/@@TAG@@/SHA256SUMS) | Published checksums |

@@STABLE_NAME_NOTE@@## Windows

1. Download the **Windows setup** executable (recommended).
2. Make sure that its SHA-256 matches the table.
3. Open the setup properties and select **Unblock** when Windows shows that option.
4. Double-click the setup executable.

This community setup is not code-signed. Windows can show **Unknown publisher**. Continue only
after the setup SHA-256 matches this page. The launcher the setup installs is not signed either,
but the setup clears the download mark from it after checking it, so Windows should not ask a
second time. If it does ask, or if an antivirus quarantines the launcher, select **More info → Run
anyway** for that file only.

The GUI selects the included portable game. You can instead use a game it finds on your machine
or select the folder yourself. **Start after installation** is selected by default. The game opens
after the map grants this installation a place. Setup creates desktop and Start Menu launch icons.
It also registers Bibites Multiverse in Windows Settings for uninstall.

The setup installs `BibitesMultiverseLauncher.exe`, and the icons open it: the window described
under *What is new*, above. Each world it creates has its own map identity, its own data folder and
its own sidecar port, and a tick box runs one session with or without a game window without
changing that world's own setting.

The same launcher's commands and console menu ship beside it as `multiverse-launcher.exe`, which is
what a script or a scheduled task should call.

The Windows complete ZIP remains available for advanced or script-based installation. It opens
the same GUI through `Install-BibitesMultiverse.cmd`. `Start-Multiverse.ps1` and
`Stop-Multiverse.ps1` are still generated for scripted use.

## Linux

1. Download and extract the **Linux complete** archive (recommended).
2. Make sure that its SHA-256 matches the table.
3. Run `./install-bibites-multiverse.sh` from the extracted directory.

The Linux installer uses the included native game and enrolls this installation automatically.
The add-on archive finds an existing itch.io game. For a private map, pass
`--join-string-file ./join.txt`.

## Known limitations

- **Linux has no launcher window.** The Linux package keeps its shell scripts:
  `./start-multiverse.sh`, `./stop-multiverse.sh`, and `--headless` on the start script.
- **Five worlds per game folder on Windows**, for the reason under *Defaults* below. The launcher
  refuses the sixth rather than starting a world that would sit off the map.
- **Neither the setup nor the launcher is code-signed**, so Windows can show *Unknown publisher*.
  Compare the SHA-256 on this page first, every time.
- **Closing the launcher window stops nothing**, deliberately. A world keeps running until you stop
  it.
- **The selected row in the world list keeps Windows' own selection colour** behind the state's
  colour. The colour of the text is the state; the pale blue behind it is Windows.

## Defaults

The installed world exchanges organisms on all four edges. It saves every 10 minutes, keeps six
saves, and saves when the game closes. It starts at ten times the game's own speed, which the
in-game speed slider still moves.

On Windows, one game folder can run five worlds at the same time. The launcher refuses to start a
sixth, because BepInEx keeps only five log files per game folder and the mod does not load in the
sixth game. If you run more than one world, give each world a different save interval, because all
of them write into the same save folder.

The public map operates from 2026-08-14 through 2026-11-14. The operator can announce an
extension before the end date.

Read the [install guide](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/install.md) or
the [join guide](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/join.md) for details.

Built @@BUILT_UTC@@ from `@@COMMIT@@`.
