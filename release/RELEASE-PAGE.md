# Bibites Multiverse `@@RELEASE@@`

Connect your Bibites world to a shared map. Organisms can move between the worlds on the map.
Your game and saves stay on your computer.

## What you need

- The Bibites `0.6.3.1` for your platform.
- A private join string from the map operator.

A join string contains the map address, your world identity, and its secret. The installer asks
for it with hidden input. Do not post it in an issue, screenshot, or log.

@@EDITION_NOTE@@

## Downloads

| Platform | File | SHA-256 |
|---|---|---|
| Windows | [`@@ZIP_NAME@@`](https://github.com/@@REPO@@/releases/download/@@TAG@@/@@ZIP_NAME@@) | `@@ZIP_SHA256@@` |
| Linux | [`@@LINUX_ZIP_NAME@@`](https://github.com/@@REPO@@/releases/download/@@TAG@@/@@LINUX_ZIP_NAME@@) | `@@LINUX_ZIP_SHA256@@` |
@@COMPLETE_DOWNLOAD_ROWS@@| All | [`SHA256SUMS`](https://github.com/@@REPO@@/releases/download/@@TAG@@/SHA256SUMS) | Published checksums |

## Windows

1. Download the Windows archive.
2. Make sure that its SHA-256 matches the table.
3. Open the archive properties and select **Unblock**. Then extract the archive.
4. Double-click `Install-BibitesMultiverse.cmd`.

The installer finds the Steam game and asks for the join string. It does not start the game.

## Linux

1. Download and extract the Linux archive.
2. Make sure that its SHA-256 matches the table.
3. Run `./install-bibites-multiverse.sh` from the extracted directory.

The Linux installer finds the native itch.io game and asks for the join string.

## Defaults

The installed world exchanges organisms on all four edges. It saves every 10 minutes, keeps six
saves, and saves when the game closes.

The public map operates from 2026-08-14 through 2026-11-14. The operator can announce an
extension before the end date.

Read the [install guide](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/install.md) or
the [join guide](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/join.md) for details.

Built @@BUILT_UTC@@ from `@@COMMIT@@`.
