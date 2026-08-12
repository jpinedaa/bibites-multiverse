# Bibites Multiverse — install kit (Linux)

**This folder joins one Bibites world to a shared map.** Organisms leave your world through its
edges and arrive in other people's; theirs arrive in yours. Your world stays on your machine and
stays yours.

**You need:** Linux, and the game's **native Linux build from itch.io**. **You do not need:** a
compiler, an SDK, a runtime, root, or anything from a developer's toolchain. Nothing here will
ever ask you to turn a security control off — no `--insecure` flag, no `curl | sh`, no skipped
certificate check, and no `sudo`. If any part of this package asks you for one of those, that is a
defect and reporting it is the right response.

**Four programs, and the installer checks for them before it does anything:** `sha256sum` and
`awk`, which are already on any machine that boots; `unzip`, which unpacks BepInEx; and `file`,
which **BepInEx's own launcher** uses to check the game binary's architecture. Without that last
one the install would look complete and the game would never start, so it is checked first, where
nothing has happened yet. On Debian or Ubuntu: `sudo apt install unzip file`.

You also need **a join string**, which the operator of a map hands you out of band. It looks
like this, on one line:

```
multiverse-join/1 wss://<relay-host>/contract-b/v4 <your-world>.<secret>
```

## Before you run anything

The release page you downloaded this from carries the archive's SHA-256 **above** the download
link. Do that first if you have not already:

```sh
sha256sum bibites-multiverse-m5.0-linux-x64.zip
```

**There is no mark of the web on Linux and this page will not invent a ritual to replace it.**
Nothing here arrives quarantined and no control has to be cleared. What the checksum is for is the
same thing it is for on Windows: knowing that the file you have is the file that was published.

The installer checks itself again anyway. Its first step verifies every file in this folder
against `MANIFEST.sha256`, and only **then** sets the executable bit — on exactly the files it
just verified, by name, and on nothing else. **That order is the point**: a permission bit granted
before a checksum is a decision nobody made.

## Install

```sh
./install-bibites-multiverse.sh
```

If your shell says *permission denied*, the archive lost the mode bits on the way to you:
`bash install-bibites-multiverse.sh` runs it once, and its own step 1 fixes the rest.

It asks for your join string with the typing hidden. **There is no option that takes the secret on
the command line**, on purpose: a value typed on a command line is in every process listing on
this machine and in your shell history. If you would rather not type it, put the one-line join
string in a file and pass `--join-string-file ./join.txt` — then delete that file.

The installer finds the game, **checks your game build against `support-matrix.json` and stops if
there is no Linux entry for it**, installs BepInEx `linux_x64` and the plugin, stores the secret
half of your join string in a file only you can read, states the settings it is giving you, and
writes `start-multiverse.sh` and `stop-multiverse.sh` beside itself.

**Where the game is.** There is no registry and no library index on this platform, so the
installer looks in the itch app's own install root (`~/.config/itch/apps/…`) and a handful of
usual places, and asks you if it finds nothing. `--game-dir '/path/to/The Bibites'` says it
outright.

**It writes to no system trust store, ever.** `--ca-file` exists for a private or LAN map whose
relay signs its own certificate, and what it does is put the authority's copy beside your data and
set `SSL_CERT_FILE` in the start script — the platform's own mechanism, for that one process.
Nothing is copied into `/etc/ssl` or `/usr/local/share/ca-certificates`, `update-ca-certificates`
is never run, and no part of this needs root.

**What a bare install does:** your world **exports on all four edges**. Nothing configured means
the whole perimeter, not silence. Every edge is a door that works both ways. The installer says
so on your screen while it runs, and `docs/participant/install.md` says so on the page.

## Start and stop

```sh
./start-multiverse.sh      # the sidecar, then the game
./stop-multiverse.sh       # the game, then the sidecar
```

`start-multiverse.sh --headless` runs the world with nothing drawn; the simulation is unchanged.
`stop-multiverse.sh --game-only` puts the world down and leaves the sidecar up, so it keeps your
place on the map and keeps taking custody of everything that arrives while you are away.

**The game is started through BepInEx's own launcher**, `./run_bepinex.sh "./The Bibites.x86_64"`,
from the game's own folder. That is the whole difference from the Windows kit's start script: the
mod framework hooks the game through `LD_PRELOAD` and a shim library rather than through a DLL the
loader picks up, so the launcher has to be the thing you run. Your settings are ordinary
environment variables and the game inherits them through it.

**`stop-multiverse.sh` asks before it insists.** It sends `SIGTERM`, waits up to twenty seconds,
and only then kills — because your world's save-on-quit runs during that shutdown, and a world
that is killed outright loses everything since its last save. A clean quit with its save has
measured at about two seconds.

**Stopping costs nothing and needs nobody.** Your place on the map is keyed on your world's
identity and never expires.

## ONE GAME INSTANCE PER GAME FOLDER

**This is the Linux trap, and it is the inverted twin of the Windows one.** Run two worlds out of
one game folder and everything works: both load the mod, both join the map, both save. What
breaks is the evidence. Every instance opens the *same* `BepInEx/LogOutput.log`, each with its own
file offset, each truncating it at launch — and the file ends as mostly NUL bytes, with one
session's header where five should be.

Windows loses one instance loudly. **Linux keeps every instance and shreds the log for all of
them at once**, so the first thing you reach for when something is wrong is the thing that is
already gone.

If you want a second world on this machine, **unpack a second copy of the game into its own
folder** and install into that. It costs disk and nothing else. `LOCAL-LOGSHRED` in
`error-taxonomy.md` has the shape of it and how to recognise a shredded log.

## Where your worlds live

Not here, and not in this install's data directory:

```
${XDG_CONFIG_HOME:-$HOME/.config}/unity3d/The Bibites/The Bibites/Savefiles
```

That is the game's own folder, on Unity's own terms. **Nothing in this package writes outside its
own directory there, and the uninstall never goes near it.** Setting `XDG_CONFIG_HOME` before the
game starts moves the whole tree — which is how a world is put on a separate disk, and it is a
thing you do to the game rather than to this mod.

## Uninstall

```sh
./uninstall-bibites-multiverse.sh --dry-run   # the ledger, changing nothing
./uninstall-bibites-multiverse.sh
```

It reads the record the installer wrote and removes **only** what is named in it, hash-checked,
printing a line per path for what it removed and what it kept. That includes the four files
BepInEx's Linux archive lays in the game's own root — `run_bepinex.sh`, `libdoorstop.so`,
`.doorstop_version` and its `changelog.txt` — but only if this installer put them there. Your
worlds and their backups are never touched. Your journal — the organisms this machine is holding
for other worlds — is kept unless you pass `--remove-world-data`.

There is no `--keep-certificate`, because there is nothing to keep: this installer never wrote to
a trust store.

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
| `install-bibites-multiverse.sh` | The installer |
| `uninstall-bibites-multiverse.sh` | The uninstaller |
| `BibitesMultiverse.dll` | The mod, a BepInEx plugin. **The same file the Windows archive ships** — it is platform-independent IL |
| `multiverse-sidecar` | The program that speaks to the map on your world's behalf. A static binary; it needs no libc of a particular vintage |
| `BepInEx_linux_x64_5.4.23.3.zip` | The mod framework, exactly as its own project publishes it |
| `support-matrix.json` | The game builds this release supports, and the words it refuses with. **The same bytes the Windows archive carries** |
| `MANIFEST.sha256` | The SHA-256 of every file above, which the installer checks first |
