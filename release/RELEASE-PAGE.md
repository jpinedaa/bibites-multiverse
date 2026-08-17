# Bibites Multiverse `@@RELEASE@@`

Connect your Bibites world to a shared map. Organisms can move between the worlds on the map.
Your game and saves stay on your computer.

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

The setup installs `BibitesMultiverseLauncher.exe`, and the icons open it. **The launcher is now a
window.** It lists every world on this computer and keeps the list up to date while they run: what
is running, whether the game's mod has really reached the map, the speed the world is achieving,
and its place on the map. Buttons start and stop a world, and one of them runs a single session
with or without a game window without changing the world's own setting. It can also create, clone
and delete worlds: each has its own map identity, its own data folder, and its own sidecar port.
Closing the window leaves the worlds running, which the window says.

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

## Defaults

The installed world exchanges organisms on all four edges. It saves every 10 minutes, keeps six
saves, and saves when the game closes.

On Windows, one game folder can run five worlds at the same time. The launcher refuses to start a
sixth, because BepInEx keeps only five log files per game folder and the mod does not load in the
sixth game. If you run more than one world, give each world a different save interval, because all
of them write into the same save folder.

The public map operates from 2026-08-14 through 2026-11-14. The operator can announce an
extension before the end date.

Read the [install guide](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/install.md) or
the [join guide](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/join.md) for details.

Built @@BUILT_UTC@@ from `@@COMMIT@@`.
