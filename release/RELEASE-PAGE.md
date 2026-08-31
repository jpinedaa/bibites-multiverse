# Bibites Multiverse `@@RELEASE@@`

Connect your Bibites world to a shared map. Organisms can move between the worlds on the map.
Your game and saves stay on your computer.

## What is new in `@@RELEASE@@`

- **Name your world, and sign it.** The installer asks two new optional questions: the handle you
  want to be known by (suggested from your account name) and your world's name (suggested
  "your handle's world"). Both are **public** — every world on the map sees them exactly as you
  type them — and nothing is published that you did not see and confirm: press Enter to take the
  suggestion, type your own, or answer `-` to publish neither. The same handle on a second
  installation groups your worlds under one keeper, and you can change or withdraw either name
  later from the launcher.
- **The map now recognizes its keepers.** The home page greets the newest worlds — name, keeper,
  position, and when they joined — and ranks keepers by the simulated time their worlds have
  contributed, all their worlds combined, including ones that have since gone dark. Your
  contribution outlives your world.
- **The population limit is sized for a shared ×10 reference, and learning runs at whatever speed
  you play.** You no longer need to run at ×10 — or at any particular target — for the limit to
  learn or to apply. Retained budget samples are repriced for the reference rather than discarded.
- **A stalled OPEN – LEARNING badge un-sticks itself.** A world that sat learning forever because
  it ran slower than its target starts sampling immediately after this update. Explicit
  `--inbound-target-time-scale` / `MULTIVERSE_INBOUND_TARGET_TIME_SCALE` choices still stand.
- **The plugin does not change, and old participants stay welcome.** This release keeps mod
  `0.6.8`, Contract A 2.4, and Contract B 4.2; the two name fields are optional additions and no
  minimum wire version moves.

## Upgrading from an earlier release

- **Stop every world before you run the setup.** Use *Stop every world* in the launcher's window,
  or `multiverse-launcher.exe stop --all`. Setup refuses while anything of this installation is
  running, and it refuses before it has changed anything.
- **Install over the top.** Your world keeps its identity, its place on the map, its saves and its
  journal.
- **You are asked the two name questions once.** An attended upgrade asks them if this
  installation never has been; an unattended or redirected run asks nothing and publishes nothing.
  An answer you already gave — a decline included — is kept, never replaced with a suggestion.
- **Do not clear the journal or admission state.** Existing queue entries, identities, saves,
  budget samples, and learned limits remain in place; the samples are repriced for the ×10
  reference on first start.
- **No admission setting is required.** The default remains non-enforcing shadow mode.

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
