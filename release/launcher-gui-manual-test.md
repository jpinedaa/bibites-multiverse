# The launcher window, on a Windows machine

**What this is.** The proof that `BibitesMultiverseLauncher.exe` — the launcher's window — does what
the launcher does. It is a by-hand and driveable script for a machine that has Windows, a game and a
map, because nothing in this repository can open a Win32 window: `walk` is Win32 only, and the
development machine has no Windows at all.

**What is already proved elsewhere, and is not repeated here.** Every decision about a world is made
in `go/internal/launcher`, and its tests run on any machine: the refusals, the slot wait, the
mod-connected wait, the mod-quit stop and its four outcomes, enrollment and its 409/429/503 wording,
and the gate before a folder is deleted (`session_test.go`, `session_linux_test.go`,
`cli_test.go`, `run_linux_test.go`). Everything about the window that is not a widget is proved by
`go/internal/launchergui/view_test.go`: the table's columns, the four states of the **On the map**
column, which buttons a state enables, the flags an edit dialog turns into a `profile set`, and the
log pane's line assembly.

**So what is left is exactly this: does the window exist, does clicking it reach the core, and does
what the core said arrive on screen.**

---

## Before you start

| Thing | Value |
|---|---|
| Package | a `0.2.8` (or later) Windows package built from `feat/launcher-gui` |
| Install root | wherever the setup put it, e.g. `%LOCALAPPDATA%\Programs\Bibites Multiverse` |
| Files that must be there | `BibitesMultiverseLauncher.exe`, `multiverse-launcher.exe`, `multiverse-sidecar.exe`, `public-map.json`, `bibites-multiverse.ico`, `profiles\default.json` |
| A second world | created during the run, so start with one |

Automation hooks, if the harness drives this rather than a person:

- The main window's caption is exactly `Bibites Multiverse` — `launchergui.WindowTitle`, and it is
  frozen for this reason.
- Every button and menu item has a **stable, unique caption**, listed in
  `go/internal/launchergui/view.go` (`ButtonStart`, `ButtonStopAll`, `ButtonDialogDelete`, …), and a
  test enforces that no two are the same string. Find them by caption; click with `BM_CLICK` or the
  MSAA/UIA `Invoke` pattern.
- They are real Win32 controls (`walk` creates `SysListView32`, `Button`, `Edit`, `msctls_statusbar32`),
  so MSAA sees the whole tree without any accessibility work of our own.
- The world list is a list view; read its rows for the assertions below.
- The log pane is a read-only multiline `Edit`: `WM_GETTEXT` is the whole session's log.

---

## 1. It opens, and it opens as a window

1. Double-click the **Bibites Multiverse** desktop icon.
2. **Assert:** a window titled `Bibites Multiverse` appears, and **no console window flashes** before
   it. A console flash means the `-H=windowsgui` link flag did not take, which the build also checks
   (`file` must say `PE32+ executable (GUI)`).
3. **Assert:** the window's icon in the taskbar and in Alt-Tab is the project icon, not the Go
   gopher. The gopher means the resource object was not generated before the build.
4. **Assert:** the controls look like the rest of Windows (a themed list view with a real header). Flat
   grey Windows-95 controls mean the Common Controls 6 manifest is missing from the resource object.
5. **Assert:** the status bar along the bottom reads
   `worlds keep running when you close this window   -   1 world(s) in <install root>`.
6. **Assert:** the log pane's first two lines are the release and the install root, then the same
   hint naming `multiverse-launcher.exe`.

## 2. The world list is the installation

1. **Assert:** one row, for the world the installer made, named `default` with a `*` in front of it.
2. **Assert:** the columns read `Save name` = the installed world, `Port` = `8787`,
   `Window` = `window`, `Sidecar` = `stopped`, `Game` = `stopped`, `On the map` = `-`,
   `Speed` = `-`, `Slot` = `-`, `Data folder` = the world's data root.
3. **Assert:** no banner is shown above the list.

## 3. Start, and the one fact that matters

1. Select the row. Click **Start**.
2. **Assert while it runs:** the buttons grey out (an action is running) and the log pane fills with
   the same lines the console prints — `sidecar started (pid N) -> wss://…`, then
   `waiting for the map to grant this world a place ...`, then `YOU ARE ON THE MAP:` and the relay's
   own granted line. Every line carries a `HH:MM:SS` stamp.
3. **Assert:** within a couple of seconds of the sidecar starting, the list's `Sidecar` column reads
   `running (pid N)` with the same pid the log named.
4. **Assert:** after the game starts, `Game` reads `running (pid N)`, and the log says
   `game started (pid N); it loads the world '<world>' by itself…`.
5. **Assert (the important one):** when the game's mod reaches the sidecar, the log says
   `the game joined the map: mod connected, speed x10.` **and** the `On the map` column changes to
   `connected (mod <version>)`. The version must be the mod's real version, not blank.
6. **Assert:** the `Speed` column becomes `x10` and then `x10 (<achieved> achieved)` once the sidecar
   has measured a span, and `Slot` becomes this world's slot number.
7. **Assert:** the buttons come back, **Start** is now greyed and **Stop** is enabled.

### 3b. The failure that must never be silent

To see the other side of step 5, make the mod not arrive — the honest way is a game folder with the
plugin removed, but any world whose game never joins will do.

1. **Assert:** the start still finishes and the window does not claim success: the log carries
   `!! THE GAME STARTED BUT ITS MOD HAS NOT REACHED THE SIDECAR after 120 s.`, the whole block under
   it (`LOCAL-CONFIGRACE`, `LOCAL-STARVATION`, the `BepInEx/LogOutput.log` path, and
   `this is a warning, not a failure`).
2. **Assert:** the `On the map` column reads `NOT CONNECTED - see the log`.
3. **Assert:** `Open the game's BepInEx log` opens Explorer with that log selected.

## 4. The per-session window override

1. Stop the world (section 5), leaving its profile at `Window` = `window`.
2. **Assert:** the second button's caption is `Start with no window (this time only)`.
3. Click it.
4. **Assert:** the game starts with no window (`-batchmode -nographics`; nothing is drawn), and the
   log says `It runs with nothing drawn. The simulation is unchanged; only the picture is gone.`
5. **Assert:** the list's `Window` column still reads `window`, and
   `multiverse-launcher.exe profile show default` still reports `headless off`. **The override must
   not have been written into the world.**
6. Stop it. Tick **headless** in **Edit settings...** and save.
7. **Assert:** the caption flips to `Start with a window (this time only)`, and using it starts a
   world with a window while the profile still says headless.

## 5. Stop, and whether anything was lost

1. With a world running — **headless**, which is the case that used to lose a save — click **Stop**.
2. **Assert:** the log says
   `stopped the game (pid N) - it was asked through the mod, and it saved and quit`, then
   `stopped the sidecar (pid N) - it was asked to close, and it closed`.
3. **Assert:** the log does **not** contain `forced immediately: it has no window`, and does not
   contain the `LOCAL-HEADLESSSTOP` note. A headless stop through this window loses nothing.
4. **Assert:** both process columns return to `stopped` within one refresh, and `On the map` to `-`.
5. Start two worlds, then click **Stop every world**.
6. **Assert:** the log carries a `--- <name>` header for each world and both are stopped.

## 6. Create another world

1. Click **Create another world...**.
2. **Assert:** the dialog opens on values that will not be refused: a name no world has, the same
   game folder, a data folder beside the first world's, and the lowest free port (`8788`).
3. **Assert:** the dialog says, before anything is agreed to, that every world takes another
   identity on the public map and that the map applies a per-address enrollment limit, and that
   deleting a world here is not leaving the map.
4. **Type `70000` into the port field** and press **Create and enroll**.
5. **Assert:** nothing is created, and the log carries the core's own words:
   `the sidecar port 70000 is outside 1024-65535`. **The map must not have been contacted** — check
   that no `enrollment-pending.json` appeared in the new data folder.
6. Repeat with a valid port and press **Create and enroll**.
7. **Assert:** the log carries `requesting a unique identity from https://…`, then
   `wrote …\profiles\<name>.json`, the world's identity (`public-…`), the relay it dials, and the
   two notes.
8. **Assert:** the list now has two rows, and the new one's `Port` is what the dialog said.
9. **Assert:** `Copy peer id` puts the new world's `public-…` identity on the clipboard.

## 7. Edit, default, clone

1. Select a world, **Edit settings...**, change `save every N minutes` to `5`, press **Save**.
2. **Assert:** the log says `wrote …\profiles\<name>.json`, and re-opening the dialog shows `5`.
3. **Assert:** an edit that changes nothing says `nothing about '<name>' was changed.` and writes
   no file.
4. **Assert:** emptying the **species that never leave** field is accepted as
   `--no-migration-exclusion` — the log shows the exclusion-policy-off warning rather than the
   refusal about an empty value.
5. Select the world that is not marked `*`, click **Set as default**.
6. **Assert:** the `*` moves, and `multiverse-launcher.exe status` (no world named) reports that
   world.
7. **Clone world...** the first world.
8. **Assert:** the clone gets a new identity, a new port, a new data folder and a new save name, and
   the log names all four.

## 8. Delete, which has to be hard

1. Select the clone. Click **Delete world...**.
2. **Assert:** the dialog carries the custody warning (*"Deleting this world here is NOT leaving the
   map"*), names the data folder, offers the checkbox
   `also remove this world's data (journal, logs, credential)…`, and asks for the world's name.
3. Press Enter without typing anything.
4. **Assert:** **Cancel** is the default button — nothing is deleted by pressing Enter.
5. Re-open it, type a **wrong** name, press **Delete this world**.
6. **Assert:** the log says `that is not '<name>'. Nothing was deleted`, and the world is still in
   the list.
7. Re-open it, tick the data checkbox, type the right name, press **Delete this world**.
8. **Assert:** the log carries the custody warning, the list of this world's own entries, `deleted
   …\profiles\<name>.json`, and either `deleted <data root>, including its journal` or a `kept <data
   root>: N thing(s) in it are not this world's…` line naming every one of them.
9. **Assert:** a running world's **Delete world...** button is greyed.

## 9. Check this world

1. Click **Check this world**.
2. **Assert:** the log carries the command line it ran (`…\multiverse-sidecar.exe --diagnose
   --data-dir … --game-dir …`) and then the diagnostic's whole output, twenty-one checks with their
   verdicts.
3. **Assert:** a diagnostic that finds a fault does not look like a launcher failure: the output is
   there, followed by `the diagnostic finished with: exit status N`.

## 10. Closing the window stops nothing

1. Start a world. Close the window with its X.
2. **Assert:** `multiverse-launcher.exe status --all` still reports the sidecar and the game running,
   with the same pids.
3. **Assert:** the game is still on screen (or still in Task Manager, if headless).
4. Re-open the window.
5. **Assert:** the list shows that world running, and `On the map` still reads
   `connected (mod …)` — the reading is taken from the sidecar, not remembered.
6. Stop it from the window.

## 11. Two windows at once

1. Open the launcher twice.
2. **Assert:** both windows list the same worlds and both refresh.
3. Click **Start** on the same world in both, as close together as you can manage.
4. **Assert:** one starts it, and the other says
   `another launcher is starting or stopping this world (…launcher.lock was taken … ago by pid N).
   Wait for it to finish` — the per-world lock, not a corrupted world. Exactly one sidecar is
   running afterwards (`status --all`, and one process in Task Manager).

## 12. The command line, and the two executables

1. In PowerShell, from the install root:

   ```powershell
   .\multiverse-launcher.exe status --all --json | ConvertFrom-Json
   ```

   **Assert:** valid JSON, `format` = `bibites-multiverse/launcher-status/1`, and every world in it.

2. **Assert:** the console menu still works — `.\multiverse-launcher.exe` with no arguments on a
   terminal draws the numbered menu, and `0` quits.

3. **The forwarding, which is compatibility for an old script:**

   ```powershell
   .\BibitesMultiverseLauncher.exe status --all --json
   $LASTEXITCODE
   ```

   **Assert:** the same JSON reaches stdout and `$LASTEXITCODE` is `0`.

   ```powershell
   .\BibitesMultiverseLauncher.exe status --profile nosuchworld
   $LASTEXITCODE
   ```

   **Assert:** the refusal reaches stderr and `$LASTEXITCODE` is `1` — the child's code, forwarded.

   ```powershell
   .\BibitesMultiverseLauncher.exe nonsense
   $LASTEXITCODE
   ```

   **Assert:** `2`, the usage code.

4. **Assert:** `.\BibitesMultiverseLauncher.exe` with **no** arguments opens the window and returns
   immediately (a shell does not wait for it) — which is the whole reason the commands are a second
   file, and the reason the documentation tells a script to call `multiverse-launcher.exe`.

5. In the window: **Worlds → Open the console launcher**.
   **Assert:** a console window opens with the numbered menu in it.

## 13. Help and about

1. **Help → About.**
   **Assert:** `Bibites Multiverse launcher 0.2.8`, the install root, the sidecar's path,
   `multiverse-launcher.exe`, `https://bibitesmultiverse.com`, and the closing hint.
2. **Help → Documentation.**
   **Assert:** the default browser opens `https://bibitesmultiverse.com`.

## 14. A broken profile is a banner, not an empty list

1. With the window closed, put a file that is not JSON at `profiles\broken.json`.
2. Open the window.
3. **Assert:** the worlds that do parse are all listed, and a red banner above the list names
   `broken.json` and says the launcher could not read it. **An installation with unreadable files
   must never look like an installation with no worlds.**
4. Remove the file. **Assert:** the banner goes on the next refresh.

## 15. The uninstall takes both executables

1. Run `Uninstall-BibitesMultiverse.ps1`.
2. **Assert:** it refuses while a world is running, naming `multiverse-launcher.exe stop --all`.
3. Stop everything, close the window, uninstall.
4. **Assert:** neither `BibitesMultiverseLauncher.exe` nor `multiverse-launcher.exe` is left in the
   application directory, and `release/test-install-uninstall.ps1` passes — it now checks both.

---

## What to report back

For each section: pass, or the exact text the log pane held and the columns the list showed. The two
answers that matter most, because no test in this repository can reach them:

1. **Does the window open with themed controls and its own icon** (sections 1.3 and 1.4)? Those are
   the resource object, and they are the one thing a missing `go generate` breaks silently.
2. **Does the `On the map` column agree with the log** (sections 3.5 and 3b.2)? That column is the
   reason this window exists.
