# Bibites Multiverse `@@RELEASE@@`

Connect your Bibites world to a shared map. Organisms can move between the worlds on the map.
Your game and saves stay on your computer.

## What is new in `@@RELEASE@@`

- **An overloaded live world no longer traps portal traffic.** When a destination refuses before
  taking custody, the sender continues in the original direction to the next compatible world it
  has not tried. The migration keeps its identity and remains bounded, so this does not create a
  duplicate or an unbounded loop.
- **Population-aware inbound admission is available.** A world can use a fixed living-population
  limit or a per-machine adaptive limit sized for a requested simulation speed. This release
  deploys adaptive estimation in shadow mode: it learns and reports the limit but does not refuse
  organisms on population yet.
- **The live and local status views explain the decision.** They show the admission mode, target
  speed, learned and effective limits, committed population, sample count, open/closed decision,
  and rejection count.
- Mod `0.6.8` reports the requested speed separately from the speed the game actually achieves,
  so a world deliberately set to ×5 is not mistaken for a machine failing to hold ×10.

## Upgrading from an earlier release

- **Stop every world before you run the setup.** Use *Stop every world* in the launcher's window,
  or `multiverse-launcher.exe stop --all`. Setup refuses while anything of this installation is
  running, and it refuses before it has changed anything.
- **Install over the top.** Your world keeps its identity, its place on the map, its saves and its
  journal.
- **No admission setting is required.** The new default is non-enforcing shadow mode. Existing
  queue pacing, saves, credentials, and journals continue unchanged.

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
or select the folder yourself. **After installation, start The Bibites, connect, and open the
launcher** is selected by default. The game starts after the map grants this installation a place.
The installed launcher then opens. Setup creates desktop and Start Menu launch icons. It also
registers Bibites Multiverse in Windows Settings for uninstall.

The setup installs `BibitesMultiverseLauncher.exe`, and the icons open it: a window listing every
world on this computer, what each one is doing and whether its mod has reached the map, with one
button to start or stop the world you selected and buttons beside it to create, clone, delete and
check one. Each world it creates has its own map identity, its own data folder and its own sidecar
port, and a tick box runs one session with or without a game window without changing that world's
own setting.

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
