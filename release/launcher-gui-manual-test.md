# The launcher window, on a Windows machine

**What this is.** The proof that `BibitesMultiverseLauncher.exe` — the launcher's window — does what
the launcher does. It is a by-hand and driveable script for a machine that has Windows, a game and a
map, because nothing in this repository can open a Win32 window: `walk` is Win32 only, and the
development machine has no Windows at all.

**What is already proved elsewhere, and is not repeated here.** Every decision about a world is made
in `go/internal/launcher`, and its tests run on any machine: the refusals, the slot wait, the
mod-connected wait, the mod-quit stop and its four outcomes, enrollment and its 409/429/503 wording,
and the gate before a folder is deleted (`session_test.go`, `session_linux_test.go`, `cli_test.go`,
`run_linux_test.go`). Everything about the window that is not a widget is proved by the build-tag-free
files of `go/internal/launchergui` — `view.go`, `details.go`, `windowstate.go` and their tests: the
state-to-words-to-colour table, the panel, which control a state enables, the flags an edit dialog
turns into a `profile set`, the progress phrases read off the core's output, and the log pane's
line assembly and scroll arithmetic.

**So what is left is exactly this: does the window exist, does clicking it reach the core, does what
the core said arrive on screen, and are the colours and the collapsing pane what they claim to be.**

> **This window was redesigned.** If you have a script from the previous round, throw it away: the
> ten-column table is now two columns and a panel, the fourteen buttons are one big one plus two
> menus, and the log pane is collapsed by default. Every caption in section 0 is new or moved.

---

## Before you start

| Thing | Value |
|---|---|
| Package | a Windows package built from this branch |
| Install root | wherever the setup put it, e.g. `%LOCALAPPDATA%\Programs\Bibites Multiverse` |
| Files that must be there | `BibitesMultiverseLauncher.exe`, `multiverse-launcher.exe`, `multiverse-sidecar.exe`, `public-map.json`, `support-matrix.json`, `bibites-multiverse.ico`, `profiles\default.json` |
| A second world | created during the run, so start with one |
| One file this window writes | `%APPDATA%\Bibites Multiverse\launcher-window.json` — its size, position and whether the details pane was open. Delete it before section 1 for a clean first run |

Automation hooks, if the harness drives this rather than a person:

- **Find the window by the PREFIX `Bibites Multiverse`, not by the whole caption.** The caption is
  now `Bibites Multiverse` when nothing is running and `Bibites Multiverse - 1 of 2 worlds running`
  when something is (`launchergui.WindowTitle` and `WindowTitleFor`). The prefix is frozen.
- Every button, checkbox and menu item has a **stable, unique caption**, listed in section 0 and in
  `go/internal/launchergui/view.go`; a test enforces that no two are the same string. Find them by
  caption; click with `BM_CLICK` or the MSAA/UIA `Invoke` pattern. One caption may appear on more
  than one control (a button and the menu item that does the same thing) — those are the same
  action and are enabled together.
- They are real Win32 controls (`walk` creates `SysListView32`, `Button`, `Edit`, `Static`,
  `msctls_progress32`, `msctls_statusbar32`), so MSAA sees the whole tree.
- The world list is a two-column list view; read its rows for the assertions below.
- The details pane is a read-only multiline `Edit` **inside a container that is hidden until
  `Show details` is pressed**. `WM_GETTEXT` on it is the whole session's log whether it is visible or
  not; `IsWindowVisible` on its parent is whether the pane is open.

---

## 0. The captions, in full

Assert each of these exists, spelled exactly like this.

**The panel (right-hand side), top to bottom**

| Caption | Control |
|---|---|
| `Start` / `Stop` | the big primary button; the caption is `Start` for a stopped world and `Stop` for a running one |
| `Run without a game window (headless)` | check box |
| `Open the data folder` | small button, beside the **Data folder** value |
| `Copy this world's identity` | small button, beside the **Map identity** value |
| `Edit settings...` | button, in the secondary row |
| `Run a health check` | button, in the same row |
| `Show details` / `Hide details` | one button, in the same row; the caption says what pressing it will do |

**Along the bottom**

| Caption | Control |
|---|---|
| `Create a world...` | button |
| `Stop every world` | button |
| `Copy the details` | button, **inside the details pane**, so it is only visible when the pane is |

**Menu bar**

| Menu | Items, in order |
|---|---|
| `World` | `Start`, `Stop`, —, `Run a health check`, —, `Edit settings...`, `Clone this world...`, `Delete this world...`, `Set as the default world`, —, `Create a world...`, `Stop every world`, —, `Refresh now`, `Quit` |
| `Open` | `Open the data folder`, `Open the logs folder`, `Open the game's own log`, —, `Open the commands window` |
| `Help` | `Documentation (bibitesmultiverse.com)`, —, `About` |

**Right-click on a world in the list**

`Start`, `Stop`, —, `Run a health check`, `Edit settings...`, `Set as the default world`, —,
`Clone this world...`, `Delete this world...`, —, `Open the data folder`, `Open the logs folder`,
`Open the game's own log`, `Copy this world's identity`

**Dialog buttons**

| Dialog | Accepting button | Other |
|---|---|---|
| Edit settings | `Save changes` | `Cancel` |
| Create a world | `Create this world` | `Show advanced settings` / `Hide advanced settings`, `Cancel` |
| Clone | `Create the copy` | `Cancel` |
| Delete | `Delete it permanently` | `Cancel`, check box `Also delete this world's own folder` |

**Fixed labels**

- List columns: `World`, `Status`.
- Panel facts, in this order: `Save name:`, `Port:`, `Speed:`, `Data folder:`, `Map identity:`.
- Details pane header: `Everything the launcher did this session, newest at the bottom:`.
- Status bar: `Your worlds keep running when you close this window   -   1 world in <install root>`
  — **`1 world`, `2 worlds`; never `world(s)`.**
- Edit dialog group boxes: `This world`, `Who may leave, and where`, `Saving`.

**Panel order, top to bottom** — assert it reads down without a hole in it:

world name (large, bold) · headline (coloured) · hint (only sometimes) · progress bar (only while
something runs) · result line (only after something ran) · **`Start`/`Stop`** · the check box,
**left-aligned under that button** · the row `Edit settings...` `Run a health check`
`Show details` · the five facts · empty space. There must be **no gap** between the secondary row
and the facts, and no control pinned to the bottom of the panel.

**Assert (the wording rule):** no caption, label, group box, check box, **tooltip, or sentence in
any dialog** contains the words **profile**, **sidecar**, **contract**, **peer**, **enroll**,
**journal**, **credential**, **BepInEx**, or any path into the source repository (**`docs/`**,
anything ending **`.md`**).

> **The one exemption: the quoted half of a failure.** A red result line is two halves — what the
> launcher was attempting, which obeys the rule, and after the colon the **core's own sentence,
> passed through exactly as it was printed**. That half is deliberately never edited: a launcher
> that paraphrased its own refusals would be one whose window and whose log disagreed about what
> went wrong, and the log is what gets pasted into a bug report. Where a core message could equally
> say "world", it now does — `validatePort` used to say *"the profile 'default' already uses port
> 8787"* and now says *"the world 'default'…"* — but where it cannot, the sentence still wins.
> **Do not report an internal word after the colon of a red result line as a failure of this rule.** Those words belong to the program and they are all still present — inside
the details pane, which is the core's own output and is deliberately left in the core's own
vocabulary. This rule covers the dialogs, which is where the previous round leaked: the create dialog
said *"the map applies a per-address enrollment limit"*, the delete dialog said *"your sidecar may
still be holding organisms it took custody of"*, and both pointed at `docs/participant/leave.md`.

**Assert (tooltips):** hovering each control in the two tables above shows a tooltip, and no tooltip
merely repeats its caption.

---

## 1. It opens, and it opens as a window

1. Delete `%APPDATA%\Bibites Multiverse\launcher-window.json` if it exists. Double-click the
   **Bibites Multiverse** desktop icon.
2. **Assert:** a window whose caption begins `Bibites Multiverse` appears, and **no console window
   flashes** before it. A console flash means the `-H=windowsgui` link flag did not take, which the
   build also checks (`file` must say `PE32+ executable (GUI)`).
3. **Assert:** the window's icon in the taskbar and in Alt-Tab is the project icon, not the Go
   gopher. The gopher means the resource object was not generated before the build.
4. **Assert:** the controls look like the rest of Windows (a themed list view with a real header,
   themed buttons). Flat grey Windows-95 controls mean the Common Controls 6 manifest is missing
   from the resource object.
5. **Assert:** the window is at least 900 x 560; try to drag it smaller and confirm it stops there.
6. **Assert (the layout):** the world list is on the LEFT with a draggable splitter beside it, the
   panel is on the RIGHT, and **the details pane is not visible at all** — no log, no monospaced
   text, nothing below the splitter but the two bottom buttons and the status bar.
7. **Assert:** the status bar reads
   `Your worlds keep running when you close this window   -   1 world in <install root>` — **not**
   `1 world(s)`.
8. **Assert (the first painted frame is already right):** with a world already running before the
   window is opened, the very first frame shows the world in the list AND the caption
   `Bibites Multiverse - 1 of N worlds running`. The bare caption with an empty list, even for a
   moment, means the first reading is being waited for rather than taken.

## 2. The world list is the installation, in two columns

1. **Assert:** one row, for the world the installer made: `World` = `* default` (the `*` marks the
   default world), `Status` = `Stopped`, drawn in **grey**.
2. **Assert (the selected row keeps its colour):** the selected row's **text** is drawn in its own
   state's colour, on both cells — not in the white the system draws on a selection. Select a green
   world and a red one in turn: each stays green and red while selected. The one row a person's eye
   is on is the one row that must not lose its signal.
   **The BACKGROUND of that row is Windows' and is not asserted here.** An earlier round of this
   document promised a pale wash of the state's own hue behind the selected row; a machine measured
   the system's selection colour, `204,232,255`, across the whole row instead, with the coloured text
   on top of it and not one pixel of the wash. walk's `TableView` overwrites whatever `StyleCell`
   returns for a selected row with the theme's own colour before it fills it (`tableview.go`,
   `NM_CUSTOMDRAW` at `CDDS_ITEMPREPAINT`), and there is no way through it short of drawing every
   cell by hand. That selection colour is the Explorer theme's pale blue rather than
   `COLOR_HIGHLIGHT`, so the state's dark colour on it is readable, which is what the assertion above
   is for. **Assert only that:** the selected row's background is a pale tint (not a saturated blue)
   and its text is the state's colour.
3. **Assert:** no banner is shown above the list.
4. **Assert (the panel):** the world's name `default` in a larger bold face; under it the headline
   `Stopped`; under that, because this is a one-world installation that is not running,
   `This is your world. Click Start to join the map.`
5. **Assert (the facts):** `Save name:` the installed world, `Port:` `8787`,
   `Speed:` **`not running`**, `Data folder:` the world's data root, `Map identity:` a `public-…`
   string. **No fact is ever a bare `-`**: a dash is a symbol somebody has to interpret, and it reads
   as a fault. A world that is up but has not reported yet says `not measured yet`.
6. **Assert (nothing is cut without saying so):** the `Data folder:` value either fits, or is
   shortened **in the middle with an ellipsis** keeping the drive and the last folder
   (`C:\Users\…\BibitesMultiverse-multi-2`). It must never simply stop mid-word where the
   `Open the data folder` button begins. Hovering it shows the whole path in a tooltip.
7. **Assert:** the big button reads `Start`, is **taller than the other buttons, in a bold face, and
   carries the accent border** Windows draws round a default button; `Run without a game window
   (headless)` is unticked and its **left edge lines up with the left edge of that button**; there is
   **no** progress bar and **no** coloured result line.

## 3. Start, and the one fact that matters

1. Select the row. Click **Start**.
2. **Assert (the panel, while it runs):** a marquee progress bar appears and the headline changes
   through these phrases, in this order, in **amber**:
   - `Starting 'default'...`
   - `Connecting this world to the map...`
   - `Waiting for the map to give this world a place...`
   - `This world has its place on the map. Starting the game...`
   - `The game is starting...`
   - `Waiting for the game to join the map (up to two minutes)...`
   - `The game joined the map.`
   (A phrase may be passed through quickly; what must not happen is the headline sitting on
   `Starting 'default'...` for the whole ninety seconds.)
3. **Assert:** while it runs the world's `Status` cell reads `Starting...` in amber, and **every**
   button and menu item that acts on a world is greyed — `Start`, `Stop`, `Edit settings...`,
   `Run a health check`, `Create a world...`, `Stop every world`, the check box, and the same items
   in both menus. `Show details` and `Copy the details` stay enabled.
4. **Assert (when it finishes):** the progress bar disappears, and a **green** line appears in the
   panel reading `Started 'default'.` — and it STAYS there until the next action.
5. **Assert (the important one):** the `Status` cell reads `On the map - speed x10` in **green**, and
   the panel headline reads `Running - on the map (place N) - speed x10`.
6. **Assert:** the `Speed:` fact becomes `x10 (target)` and then `x10 (target), x<achieved>
   (achieved)` once the sidecar has measured a span.
7. **Assert:** the big button's caption has flipped to `Stop`.
8. **Assert (the title bar):** the caption is now `Bibites Multiverse - 1 of 1 worlds running`.
9. **Assert:** the details pane is **still closed**. A start that worked says so in one green line
   and does not open a log at anybody.

### 3b. The failure that must never be silent

To see the other side of step 5, make the mod not arrive — the honest way is a game folder with the
plugin removed, but any world whose game never joins will do.

1. **Assert:** the details pane **opens by itself**, without anybody pressing `Show details`, at the
   moment the core prints `!! THE GAME STARTED BUT ITS MOD HAS NOT REACHED THE SIDECAR after 120 s.`
   — and the pane is scrolled to that line.
2. **Assert:** the pane carries the whole block under it (`LOCAL-CONFIGRACE`, `LOCAL-STARVATION`, the
   `BepInEx/LogOutput.log` path, and `this is a warning, not a failure`).
3. **Assert:** the `Status` cell reads `NOT on the map` in **red**, and the panel headline reads
   `Running, but NOT on the map - see the details`, also red, with the hint
   `Nothing is reaching the map from this world. Open the details below for what to do.`
4. **Assert:** `Open` → `Open the game's own log` opens Explorer with that log selected.

### 3c. A refusal says why IN THE PANEL, in the core's own words

**This has failed twice, for two different reasons, so drive all three rows.** First the line was
picked up off the pane's own hundred-millisecond batch, by which time the action had already
finished — it is now taken on the goroutine that writes it, which fixed the **start** case. Then the
**create** and **delete** cases were still silent: their results have no world of their own to sit
beside, and a previous `Stopped every world.` — written against *every* world — was being preferred
over them, so the newest and only useful line in the window was invisible behind an older success.
Results are now ordered by **recency**, not by how specific they are.

Drive all three:

| Make it happen | The panel's red line must contain |
|---|---|
| Start the same world in two windows at once, or hold its `launcher.lock` | `Could not start 'default': another launcher is starting or stopping this world (…). Wait for it to finish` |
| `Create a world...` with a port another world already has | `'world2' was not created: the world 'default' already uses port 8787. Every world needs its own` |
| `Delete this world...` and type the wrong name | `'multi-2' was not deleted: that is not 'multi-2'. Nothing was deleted` |

1. **Assert (it is RED):** measure it. The result line must be drawn in `RGB(176, 0, 0)` on a
   failure and `RGB(0, 112, 48)` on a success. Check both directions: run something that succeeds,
   then something that fails, and confirm the same line changes colour.

   **HOW TO MEASURE IT — take a screen copy, not a `PrintWindow`.** A walk static draws its glyphs in
   a *child* window, and `PrintWindow` does not render that child: a capture taken that way shows an
   empty rectangle where the result line is, and a harness that trusted it would report "no coloured
   pixels" whatever the program did. Bring the window to the front and use
   `Graphics.CopyFromScreen` over `GetWindowRect` (`Save-LauncherShot -FromScreen` in the harness),
   then crop the label's own rectangle out of it and count the pixels near each of the four colours.
   Count `RGB(0,0,0)` too: **near-black is the failure**, and it is what this line was in every state
   for two rounds. Note that ClearType puts orange fringes on near-black glyphs, so a loose tolerance
   reports amber pixels on a black label — the counts that decide it are the exact colour's and
   near-black's.

   For the record, from a run on the machine (`RGB` targets within 60, whole-label crops):

   | state | label | red | green | amber | grey | near-black |
   |---|---|---|---|---|---|---|
   | stopped | headline `Stopped` | 0 | 0 | 0 | **183** | 0 |
   | running | headline `Running - on the map…` | 0 | **572** | 0 | 110 | 0 |
   | working | headline `Checking 'default'...` | 0 | 0 | **351** | 0 | 0 |
   | one world, stopped | hint `This is your world…` | 0 | 0 | 0 | **673** | 0 |
   | success | result `The health check found no faults…` | 0 | **487** | 0 | 491 | 0 |
   | refusal | result `'multi-2' was not deleted: …` | **1291** | 0 | 68 | 0 | 0 |
   | broken profile | banner | **3991** | 0 | 308 | 0 | 0 |

   Any of those with a near-black count and a zero colour count is the bug back again.
2. **Assert:** each red line carries **both** halves — what was being attempted, and the core's own
   sentence after a colon. A line ending `. The details below say why.` means the core's sentence was
   not captured, and is a failure of this section even though the pane has the sentence in it. **An
   empty result line, or the previous action's line still sitting there, is the other failure** and
   is what the create and delete rows caught last round.
3. **Assert:** the details pane opened by itself in each case, and the same sentence is in it.
3b. **The exact sequence that failed:** press **Stop every world** and let it finish, so every world
   shows a green `Stopped every world.` Now open **Create a world...** and give it a port another
   world already has. **Assert:** the moment it is refused, **every** world in the list shows the red
   `'world2' was not created: ...` — `Stopped every world.` must be gone from all of them, because it
   has been overtaken. Repeat with a wrong-name delete.
3c. **Assert (nothing comes back from the dead):** after that, start one world. Its own green line
   appears; the OTHER world goes **blank** rather than reverting to `Stopped every world.`, which was
   overtaken two actions ago.
4. **Assert:** the sentence quoted is the **refusal**, not the last line of the block explaining it.
   The delete case proves this: the custody warning is printed first and is indented, and the
   flush-left refusal below it is the one that must be quoted.

### 3d. The result line belongs to ONE world

**This also failed last round:** a health check on `multi-2` left
`The health check found no faults…` in the panel, and selecting `default` then showed default's
headline `Stopped` stacked straight on top of multi-2's result, as though the two were one world's.

1. With two worlds, select `multi-2` and press **Run a health check**. Wait for it to finish.
2. **Assert:** `multi-2`'s panel carries the green result line.
3. Select `default`.
4. **Assert:** `default`'s panel shows its headline and **no result line at all** — nothing has
   happened to `default` yet.
5. Select `multi-2` again. **Assert:** its result line is still there, unchanged.
6. Start `default`. **Assert:** `default` now shows `Started 'default'.`, and selecting `multi-2`
   still shows its own health-check line. Two worlds, two results, neither borrowed.
7. Press **Stop every world**. **Assert:** when it finishes, `Stopped every world.` is shown beside
   **both** worlds — that action really is about all of them.
8. Press **Create a world...** and let it fail (a taken port). **Assert:** the red line is shown
   **whatever world is selected**: the world it names does not exist and has no row to live beside,
   and it is the line a person most needs to read.
9. **Assert:** while any action is running, **no** result line is shown on any world — the panel is
   showing what is happening instead.
10. **Assert (newest wins):** a world's own result is shown when it is the newest thing that
    happened; a result with no world of its own — a refused create, any delete — is shown on every
    world while IT is the newest thing. Never the older of the two.

## 4. The window setting is a setting now, and it persists

The per-session `Start with no window (this time only)` button is **gone from the window**. The
override lives on the command line (`multiverse-launcher.exe start --headless` / `--no-headless`),
which is where a one-off belongs.

1. With the world stopped, tick **`Run without a game window (headless)`**.
2. **Assert:** the check box **stays ticked** — it must not flicker back to unticked while the edit
   runs — a spinning bar appears, the headline reads
   `Setting 'default' to run without a game window...`, and a green line follows:
   `'default' will run without a game window from now on.`
3. **Assert:** `multiverse-launcher.exe profile show default` now reports `headless on`. **The
   window wrote the world**; this is not a mode the window remembers.
4. Close the window and re-open it. **Assert:** the box is still ticked.
5. Click **Start**. **Assert:** the game starts with nothing drawn, and the details pane carries
   `It runs with nothing drawn. The simulation is unchanged; only the picture is gone.`
6. Untick it while the world is running. **Assert:** it is accepted (the check box is not greyed),
   the green line reads `'default' will run with a game window from now on.`, and the running game
   is unaffected — the change takes effect at the next start, which is what the tooltip says.

## 5. Stop, and whether anything was lost

1. With a world running — **headless**, which is the case that used to lose a save — click **Stop**.
   There is **no confirmation dialog**, and that is deliberate: a stop asks the world to save and
   quit, so it costs nothing.
2. **Assert (the phrases):** `Stopping 'default' - asking it to save and quit...`, then
   `The world is saving and shutting down...`, then
   `The game has stopped. Closing the link to the map...`.
3. **Assert:** the details pane carries
   `stopped the game (pid N) - it was asked through the mod, and it saved and quit`, then a sidecar
   line reading `stopped the sidecar (pid N) - ended directly; a sidecar keeps nothing unsaved - its
   journal is written as it goes`.
4. **Assert:** the pane does **not** contain `forced immediately: it has no window`, and does not
   contain the `LOCAL-HEADLESSSTOP` note. A headless stop through this window loses nothing. (If it
   ever did, the pane would open by itself — `LOCAL-HEADLESSSTOP` is one of the alarm lines.)
5. **Assert:** a **green** `Stopped 'default'.` stays in the panel; the `Status` cell returns to
   `Stopped` in grey within one refresh; the button reads `Start` again; the title bar drops back to
   `Bibites Multiverse`.
6. **Keyboard and mouse:** select a stopped world and press **Enter**. **Assert:** it starts.
   Double-click a running world. **Assert:** it stops. Both do exactly what the big button does.
7. Start two worlds, then click **Stop every world**.
8. **Assert:** BOTH rows read `Stopping...` in amber at once, the headline names each world in turn
   (`Stopping 'default'...`, `Stopping 'second'...`), the pane carries a `--- <name>` header for
   each, and the result is `Stopped every world.`

## 6. Create a world — a name, and nothing else unless asked

1. Click **Create a world...**.
2. **Assert:** the dialog is titled `Create a world on this computer` and shows, above everything,
   one plain sentence about what a world is. The ONLY field is `A name for the new world`,
   pre-filled with a name no world has.
3. **Assert:** the identity and leaving notes are immediately **above** the `Create this world`
   button, not at the top, and both are in plain words with **no repository path in them**:
   *"the map limits how many worlds one address may create in a short time"* and
   *"Deleting a world here does not take it off the map. See bibitesmultiverse.com for how to leave
   it properly."* The words *enrollment*, *per-address*, *sidecar* and `docs/participant/leave.md`
   must not appear.
4. Press **Show advanced settings**.
5. **Assert:** the caption flips to `Hide advanced settings` and five more fields appear —
   `Its save name`, `Its port` (the lowest free one, `8788`), `Its own folder` (beside the first
   world's), `The folder the game is installed in` (the same as the existing world's), and
   `Run without a game window (headless)`.
5b. **Assert:** every field's caption is left-aligned on **one** column — including
   `A name for the new world`, which is above the advanced block and was fourteen pixels to the right
   of everything under it last round — and the game folder is readable rather than cut off at the
   edge of the dialog. Read the left edge of the LINE EDITS too: all six must share it.
6. **Type `70000` into `Its port`** and press **Create this world**.
7. **Assert:** nothing is created; the details pane opens by itself and the panel's red result line
   quotes the core: `'world2' was not created: the sidecar port 70000 is outside 1024-65535`. **The
   map must not have been contacted** — check that no `enrollment-pending.json` appeared in the new
   data folder. (See section 3c: a line ending `. The details below say why.` fails this step.)
8. Repeat with a valid port and press **Create this world**.
9. **Assert (step two is the window, not a second dialog):** the dialog closes at once and the PANEL
   shows `Creating 'world2' - asking the map for a new identity...` with a spinning bar, then
   `Asking the map for a new identity...`, then a green
   `Created 'world2'. It has its own identity on the map.`
10. **Assert:** the details pane carries `requesting a unique identity from https://…`, then
    `wrote …\profiles\world2.json`, the world's `public-…` identity, and the two notes.
11. **Assert:** the list now has two rows and the new one's `Port:` fact is what the dialog said.
12. Select it and press **Copy this world's identity**.
    **Assert:** its `public-…` identity is on the clipboard, and the panel says
    `Copied this world's identity on the map.`
13. **Assert:** `Open the data folder` opens that world's own folder in Explorer.

## 7. Edit, default, clone

1. Select a world, **Edit settings...**.
2. **Assert:** the fields are in three labelled groups — `This world`, `Who may leave, and where`,
   `Saving` — and each field carries a tooltip. **Every field in all three groups starts at the same
   left edge**; three groups sizing their own label column put the fields at three different X
   positions last round.
3. **Assert (which values are defaults):** beside `Port` the grey note reads `(the default)` for
   `8787` and `(default: 8787)` for anything else; the same for `Edges organisms may cross`,
   `Species that never leave`, `Save every N minutes` and `Saves kept`.
4. Change `Save every N minutes` to `5`, press **Save changes**.
5. **Assert:** a green `Saved the settings for 'default'.`, the pane says
   `wrote …\profiles\default.json`, and re-opening the dialog shows `5` with `(default: 10)`.
6. **Assert:** an edit that changes nothing leaves `Nothing about 'default' was changed.` in the
   panel and writes no file.
7. **Assert:** emptying `Species that never leave` is accepted as `--no-migration-exclusion` — the
   pane shows the exclusion-policy-off warning rather than the refusal about an empty value.
8. Press **Escape** in the dialog. **Assert:** it closes and nothing is written.
9. Select the world that is not marked `*`, `World` → **Set as the default world**.
10. **Assert:** the `*` moves, the result reads `'second' is now the default world.`, and
    `multiverse-launcher.exe status` (no world named) reports that world.
11. `World` → **Clone this world...** on the first world.
12. **Assert:** the dialog is titled `Make a copy of the world '<name>'`, has one field
    `A name for the copy`, and its button is `Create the copy`.
13. **Assert:** the clone gets a new identity, a new port, a new data folder and a new save name; the
    pane names all four and the result reads `Copied 'default' to 'world3'.`

## 8. Delete, which has to be hard

1. Select the clone. `World` → **Delete this world...**.
2. **Assert:** the dialog carries the custody warning as flowing text — not indented like terminal
   output — offers the check box `Also delete this world's own folder`, names the folder, and says in
   plain words that **the game's own save file is not in it and is never touched**.
   **Assert the wording**, which is the whole of what a person has to act on: it says deleting here
   *"does NOT take it off the map"*, that this world *"may also still be holding creatures that were
   on their way somewhere else, and only this computer can pass them on"*, that the fix is to
   *"Start it once and let it finish doing that"*, and it points at `bibitesmultiverse.com`. The
   words *sidecar*, *custody*, *credential*, *journal* and `docs/participant/leave.md` must not
   appear.
3. **Assert:** the field reads `Type <name> to confirm`.
4. Press Enter without typing anything.
5. **Assert:** **Cancel** is the default button — nothing is deleted by pressing Enter.
6. Re-open it, type a **wrong** name, press **Delete it permanently**.
7. **Assert:** the red result line quotes the core — `'world3' was not deleted: that is not
   'world3'. Nothing was deleted` — the pane is open, and the world is still in the list. The
   indented custody warning printed above that refusal must **not** be what gets quoted.
8. Re-open it, tick the check box, type the right name, press **Delete it permanently**.
9. **Assert:** the pane carries the custody warning, this world's own entries, `deleted
   …\profiles\world3.json`, and either `deleted <data root>, including its journal` or a
   `kept <data root>: N thing(s) in it are not this world's…` line naming every one of them. The
   result is a green `Deleted 'world3'.`
10. **Assert:** for a **running** world, `Delete this world...` is greyed in both the `World` menu
    and the context menu.

## 9. Run a health check

1. Click **Run a health check**.
2. **Assert:** the details pane **opens by itself** — a report nobody can see is not a report — and
   the panel shows `Checking 'default'...` with a spinning bar.
3. **Assert:** the pane carries the command line it ran, and it names **this world's map**, not only
   its folders:

   ```
   …\multiverse-sidecar.exe --diagnose --relay wss://… --data-dir … --credential-file …\peer-secret.txt --game-dir …
   ```

   `--relay` and `--credential-file` are the whole point of this assertion. Without them the sidecar
   falls back to its own default relay, `ws://127.0.0.1:8795/contract-b/v4`, and a perfectly healthy
   world is told `FAIL relay-reachable` and `FAIL credential`.
4. **Assert:** then the diagnostic's whole output, each check with its verdict. On a healthy world on
   the public map that is **all passes and exit 0** — the run this was written from reported
   `16 PASS` — and the panel's result line is a green
   `The health check found no faults. Its whole report is in the details.` In particular:
   - `relay-reachable` and `credential` **pass**;
   - `game-version` names the build rather than answering `UNKNOWN …`. That answer means
     `support-matrix.json` is missing from the install root.
5. **Assert:** a diagnostic that finds a real fault does not look like a launcher failure: the whole
   output is there, followed by `the diagnostic finished with: exit status N`, and the result line is
   the red `The health check found a fault. Its whole report is in the details.` — it must **not**
   quote `exit status N` at the participant.

## 10. The details pane: collapsed, opened, and it leaves a reader alone

**This section is the one that regressed on a real machine, so drive it exactly.**

1. With the pane closed, click **Show details**.
2. **Assert:** the caption flips to `Hide details`, a monospaced pane appears below the splitter with
   `Everything the launcher did this session, newest at the bottom:` above it and
   `Copy the details` beside that, and the newest line is on screen.
2b. **Assert (it is a pane, not a slot):** it is **at least ten lines tall**, and on a window with
   room to spare it takes something like a third of the height. Three lines — which is what it
   opened as on an 840-pixel window last round — is a log nobody can read. Measure it: shrink the
   window to its minimum (900 x 560) and the pane must still be at least ten lines.
2c. **Assert (it is draggable):** there is a divider between the panel above and the pane below, and
   dragging it up and down resizes both. The world list and the panel above shrink to give it room.
   There is a second divider between the world list and the panel; dragging that resizes those two,
   and **must not** move the details divider.
3. **Assert:** its first two lines are the release and the install root, then the hint naming
   `multiverse-launcher.exe`. Every line carries a `HH:MM:SS` stamp, and each action is preceded by
   a `> ` heading (`> start default`, `> stop default`, `> check default`,
   `> edit default (--headless)`, `> create world2 and enroll a new identity on the map`).
4. Start a world so it prints for a minute, and **assert (it follows):** with nobody touching it, the
   last line on screen is the last line printed. Drive it with `EM_GETLINECOUNT` and
   `EM_GETFIRSTVISIBLELINE`: the first visible line must be within a screenful of the line count.
5. **Now the regression.** While that start is still printing, send the pane `WM_VSCROLL` `SB_TOP`
   (or scroll to the top by hand). Read `EM_GETFIRSTVISIBLELINE` — call it `before`.
6. Wait for at least ten more lines to arrive (watch `EM_GETLINECOUNT` grow).
7. **Assert (the one that failed at `81af447`):** `EM_GETFIRSTVISIBLELINE` is **still `before`**. A
   machine previously measured it jumping from `0` to `286` on the very next appended line: walk's
   `AppendText` replaces the selection at the end of the document and the EDIT control scrolls the
   caret into view as part of that, before this program is consulted. The window now reads the first
   visible line on both sides of the append and scrolls the reader back with `EM_LINESCROLL`
   (`launchergui.LinesToRestore`).
8. **Assert:** while that happens the pane does not flicker — the append and the correction are one
   redraw (`WM_SETREDRAW`), and the vertical scroll bar is redrawn with the frame, so it is not left
   at a stale position.
9. Scroll back to the bottom. **Assert:** it follows again.
10. Press **Copy the details**. **Assert:** the whole pane is on the clipboard and the panel says
    `Copied the details to the clipboard.`
11. Press **Hide details**. **Assert:** the pane disappears **and its divider with it**, the worlds
    and the panel take the whole space, and the caption is `Show details` again.
12. **Assert (no orphan divider), which failed last round:** with the pane hidden there is **exactly
    one** divider left in the window — the vertical one between the world list and the panel. There
    must be **no horizontal bar** anywhere: last round a nine-pixel-tall, window-wide handle stayed
    behind under the world list, and dragging it resized a pane nobody could see. Sweep the pointer
    along the bottom of the window and confirm the cursor never becomes a north-south resize arrow.
13. Show and hide the pane a few times. **Assert:** the divider comes back with the pane every time
    and goes away with it every time.

## 11. It remembers where it was

1. With the details pane **open**, move and resize the window, **drag the divider so the pane is
   about half the window**, then close it with its X.
2. **Assert:** `%APPDATA%\Bibites Multiverse\launcher-window.json` exists, carries
   `"format": "bibites-multiverse/launcher-window/1"`, and its `x`, `y`, `width`, `height` and
   `details` match what you left. **Assert:** it also carries a `"split"` object with one entry per
   divider, named `"details"` and `"worlds"`, each holding two pixel sizes. **The names are this
   program's**, not walk's internal widget path — a key like `"main/clientComposite/details"` in a
   file a participant can open is a leak of another library's furniture, and is what this round
   replaced.
3. Re-open the window. **Assert:** it opens at that size and position, with the details pane open
   **and the divider where you left it**, within a pixel or two. Measure it: the world list's own
   width (`GetWindowRect` on the `SysListView32`) and the details pane's height before closing and
   after re-opening. **This failed silently for a round** — the positions were written to the file
   and read back into walk's settings, and nothing ever handed them to a splitter, because
   `MainWindow.RestoreState` returns before it descends into its children when the form itself has no
   stored state. A file that says `"worlds": "345 1216"` and a window that opens with the list at its
   default width is that bug.
3b. **Assert:** there is **no second preferences file** — nothing under
   `%APPDATA%\<anything else>` and no `.ini` anywhere. One file holds the lot.
4. Maximise it, close it, re-open it. **Assert:** it comes back maximised, and un-maximising it
   returns it to the size from step 1 — the file keeps the restored rectangle, not the maximised one.
5. Edit the file to `"x": 9000` and re-open. **Assert:** the window opens at its default size on the
   screen rather than off the edge of it.
6. Replace the file's contents with `not json` and re-open. **Assert:** the window opens normally and
   says nothing about it.
6b. Delete the whole `"split"` object from a good file and re-open. **Assert:** the window opens with
   sensible default dividers — the field is additive, and a file from the previous build must still
   work.
6c. Open the window, **never** press `Show details`, and close it. **Assert:** the `"split"` entry
   for `"details"` never contains a `0`, and its **second** number — the pane's own height — is
   unchanged from what the file said before. (The first number is the half above the divider, which
   is the whole splitter while the pane is hidden, so it does grow: a file holding
   `"details": "600 300"` comes back as `"866 300"` on a 1050-pixel window, and that is correct.) A
   hidden splitter child measures zero, and a zero saved here would re-open the pane at no height
   with its divider on the bottom edge of the window.
6d. **The upgrade, which left two spellings behind last round.** Put a file holding the OLD key
   names in place and open the window:

   ```json
   { "format": "bibites-multiverse/launcher-window/1", "x": 60, "y": 60,
     "width": 1200, "height": 800, "details": true,
     "split": { "main/clientComposite/details": "700 200",
                "main/clientComposite/details/worlds": "280 900" } }
   ```

   **Assert:** the window opens with the dividers where that file put them — the positions are
   migrated, not thrown away — and after closing it the file holds **only** `"details"` and
   `"worlds"`. No key containing a `/` may survive, and no stale pair may sit beside the new one.
   **Give that file the size the window will actually keep**, or the assertion cannot be exact: this
   window's minimum is 900x610 at 96 dpi, so on a 150% display a file asking for 1200x800 opens at
   1350x915 and the splitter redistributes the extra space. At a size the window keeps, the numbers
   are exact — `"main/clientComposite/details/worlds": "615 946"` drew a 613-pixel world list, the
   same pixel a drag had left it on.
7. **Assert:** nothing was written into `profiles\` — a stray `.json` in there is read as a world and
   would raise the red banner (section 14).
8. **Assert:** `Help` → `About` names that file's path.

## 12. Closing the window stops nothing

1. Start a world. Close the window with its X.
2. **Assert:** `multiverse-launcher.exe status --all` still reports the sidecar and the game running,
   with the same pids.
3. **Assert:** the game is still on screen (or still in Task Manager, if headless).
4. Re-open the window.
5. **Assert:** the list shows that world `On the map - speed x10` in green — the reading is taken
   from the sidecar, not remembered — and the title bar says `1 of N worlds running`.
6. Stop it from the window.

## 13. Two windows at once

1. Open the launcher twice.
2. **Assert:** both windows list the same worlds and both refresh.
3. Click **Start** on the same world in both, as close together as you can manage.
4. **Assert:** one starts it, and the other's panel shows a red
   `Could not start '<name>': another launcher is starting or stopping this world (…launcher.lock
   was taken … ago by pid N). Wait for it to finish` with its details pane opened by itself — the
   per-world lock, not a corrupted world. Exactly one sidecar is running afterwards (`status --all`,
   and one process in Task Manager).

## 14. A broken file is a banner, not an empty list

1. With the window closed, put a file that is not JSON at `profiles\broken.json`.
2. Open the window.
3. **Assert:** the worlds that do parse are all listed, and a red banner above the list names
   `broken.json`, says the launcher could not read it, **and says what to do**:
   `Run the installer again, or move the named file out of that folder.`
4. Remove the file. **Assert:** the banner goes on the next refresh.
5. Move every profile out of `profiles\` and re-open. **Assert:** the banner reads
   `There are no worlds on this computer yet. Click 'Create a world...' to make one, or run the
   installer again.`, the panel says the same thing, and `Create a world...` is the only enabled
   action.

## 15. The command line, and the two executables

1. In PowerShell, from the install root:

   ```powershell
   .\multiverse-launcher.exe status --all --json | ConvertFrom-Json
   ```

   **Assert:** valid JSON, `format` = `bibites-multiverse/launcher-status/1`, and every world in it.

2. **Assert:** the console menu still works — `.\multiverse-launcher.exe` with no arguments on a
   terminal draws the numbered menu, and `0` quits.

3. **Assert (the override the window dropped is still on the command line):**
   `.\multiverse-launcher.exe start default --no-headless` starts a world with a window even though
   its profile says headless, and does not write the profile.

4. **The forwarding, which is compatibility for an old script:**

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

5. **Assert:** `.\BibitesMultiverseLauncher.exe` with **no** arguments opens the window and returns
   immediately (a shell does not wait for it).

6. In the window: `Open` → **Open the commands window**.
   **Assert:** a console window opens with the numbered menu in it.

## 16. Help and about

1. `Help` → **About**.
   **Assert:** `Bibites Multiverse launcher <this release>` — the same string
   `multiverse-launcher.exe version` prints — the install root, the sidecar's path,
   `multiverse-launcher.exe`, the window-position file's path,
   `https://bibitesmultiverse.com`, and the closing hint.
2. `Help` → **Documentation**.
   **Assert:** the default browser opens `https://bibitesmultiverse.com`.

## 17. The uninstall takes both executables

1. Run `Uninstall-BibitesMultiverse.ps1`.
2. **Assert:** it refuses while a world is running, naming `multiverse-launcher.exe stop --all`.
3. Stop everything, close the window, uninstall.
4. **Assert:** neither `BibitesMultiverseLauncher.exe` nor `multiverse-launcher.exe` is left in the
   application directory, and `release/test-install-uninstall.ps1` passes — it now checks both.

---

## The state table, for reference

Every row is a different sentence AND a different colour. Assert the colour by reading the list
item's text colour (`NM_CUSTOMDRAW` sets it per cell) or by eye.

| The world | `Status` column | Panel headline | Colour |
|---|---|---|---|
| nothing running | `Stopped` | `Stopped` | grey `RGB(96,96,96)` |
| an action of the window's is running on it | `Starting...` / `Stopping...` / `Creating...` / `Copying...` / `Deleting...` / `Checking...` / `Saving...` | the live progress phrase | amber `RGB(160,96,0)` |
| link up, nothing answering yet | `Starting...` | `Starting - waiting for the map to answer...` | amber |
| on the map, no game yet | `Waiting for the game` | `On the map (place N), waiting for the game to join` | amber |
| **on the map with a game behind it** | `On the map - speed x10` | `Running - on the map (place N) - speed x10` | **green `RGB(0,112,48)`** |
| **game running, never joined the map** | `NOT on the map` | `Running, but NOT on the map - see the details` | **red `RGB(176,0,0)`** |
| game running with nothing holding its place | `NOT on the map` | `The game is running, but this world has no link to the map - see the details` | red |

**The selected row keeps its colour** — on its TEXT. Both its cells are drawn in the state's own
colour rather than in the white a selection usually gets, because the first attempt at this
suppressed the colour on exactly the row a person's eye is on, so selecting the world you were
worried about made its red go away. Its BACKGROUND is Windows' own selection colour and cannot be
anything else: walk's `TableView` overwrites the background a `StyleCell` returns for a selected row
before it fills it. See section 2.2.

---

## What to report back

For each section: pass, or the exact text the panel held, the `Status` cell's words and colour, and
the details pane's contents. The answers that matter most, because no test in this repository can
reach them:

1. **Does the window open with themed controls and its own icon** (sections 1.3 and 1.4)? Those are
   the resource object, and they are the one thing a missing `go generate` breaks silently.
2. **Does the `Status` column agree with the details pane** (sections 3.5 and 3b.3)? That
   distinction is the reason this window exists.
3. **Does the details pane leave a reader where they put themselves** (section 10.7)? Measurable to
   the line.
4. **Does a refusal reach the panel in the core's own words** (section 3c)? All three paths, and the
   exact red line for each.
5. **Does a result stay with the world it is about, and does the newest one win** (sections 3d and
   3c.2b)? The create and delete refusals are the two that have been silent.
6. **Is there exactly one divider on screen when the details pane is hidden** (section 10.12)?
7. **Is the result line actually coloured** (section 3c.1)? Measure the pixels; this is the one
   thing that has looked right and been wrong.
8. **Does anything in the window or its dialogs still use an internal word** (section 0's wording
   rule, and its one exemption)?
