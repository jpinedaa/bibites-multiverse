# Bibites Multiverse — install kit (Linux)

**This folder joins one Bibites world to a shared map.** Organisms leave your world through its
edges and arrive in other people's; theirs arrive in yours. Your world stays on your machine and
stays yours.

**You need:** Linux. The recommended complete package includes the authorized native game and
uses it automatically. An add-on package can find a supported itch.io copy that you already have.
Both packages include the public join configuration and create a unique public-map identity.
**You do not need:** a join string, compiler, SDK, runtime, or root.

The installer does not turn a security control off. It uses no `--insecure` flag, `curl | sh`,
skipped certificate check, or `sudo`. Report any installer instruction that asks for one of these
actions.

**Five programs are required:** `sha256sum`, `awk`, `unzip`, `file`, and `curl`. The installer
checks them before it changes the game. BepInEx uses `file` to read the game architecture. The
installer uses `curl` for one HTTPS enrollment request. On Debian or Ubuntu, run
`sudo apt install unzip file curl` if one is absent.

For a private map, use **a join string** that its operator gives you out of band. It looks like
this, on one line:

```
multiverse-join/1 wss://<relay-host>/contract-b/v4 <your-world>.<secret>
```

## Before you run anything

The release page you downloaded this from carries the archive's SHA-256 **above** the download
link. Do that first if you have not already:

```sh
sha256sum bibites-multiverse-0.3.3-linux-x64-complete.zip
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

The default install reads `public-map.json`. It creates a random UUID and secret on this computer.
Then it enrolls the identity over HTTPS. The installer keeps a private pending record until the
install record is complete. If a response is lost, run the installer again. It retries the same
identity.

For a private map, put the one-line join string in a file. Pass
`--join-string-file ./join.txt`, and delete that file after installation. **There is no option
that takes the secret on the command line.** A command-line secret appears in process listings
and shell history.

**Installing again over the same data root keeps that world.** An upgrade or a repair is not a new
world: step 6 reads the identity already in the data root — from `install-record.json`, from a
pending enrollment record carrying that same secret, from the previous `start-multiverse.sh`, or
from `data/peer-id` and `data/relay-url` — says *"reusing the map identity already in …"*, and
leaves `peer-secret.txt` untouched. It asks the map for nothing and spends no second place on it,
and an adopted world keeps its own relay whether that is the public map or a private one.

**The last place it looks is the sidecar's own log.** Every line the sidecar writes carries
`peer=<identity>`, and its startup line carries `relay=` beside it, so `logs/sidecar.log` and the
rotated files beside it still say which world a folder is when nothing else does. The installer
reads them itself and prints the line it took the identity from. Two worlds in those logs means the
folder has run two: it lists them with where each was found and asks you to put the right one into
`data/peer-id` rather than choosing for you. The kit folders it searches are bounded and named —
this one, the data root, and the data root's parent, because a kit is usually unpacked beside the
data it writes.

**A secret no world ever used is an orphan, not a question.** If an earlier install got a
credential and stopped before the world ever ran — nothing inside `<data root>/data`, no `peer=` in
a sidecar log — that secret belongs to no world and holds no place on the map: the installer renames
it to `peer-secret.txt.<utc>.orphan`, keeps it, and enrolls a new identity.

**A secret is replaced only when something vouches for the world it belongs to** — this data
root's own install record, or a pending record carrying that very secret — and the replaced one is
kept beside the new file as `peer-secret.txt.<utc>.old`. A claim that only an ordinary text file
makes is enough to keep a world and never enough to destroy one. Everything else stops with
`INS-ENROLL` and changes nothing: a `peer-secret.txt` no file on this machine can name, a file that
is not a credential, a `--relay-url` that would point an adopted world at another map. Every refusal
prints the files to look in and the ways on.

**A different identity over this world takes `--replace-world-identity`.** A slot handover mints
exactly that — a new identity with a fresh credential, rebound to your old slot and position, which
is the map's only credential recovery — and a borrowed or mistyped join string looks the same from
here. The switch says which it is, and the old name is kept in `data/peer-id.previous`. **If the
secret is gone and the name is not** — what an uninstall from a release before this one left — the
installer stops and prints the world's identity: ask its operator for that handover, or use the
switch alone to take a new identity with no place, which is a second world on the map and leaves the
old one dark until its operator releases it.

The same installer handles both editions. In an add-on archive it finds the game or accepts
`--game-dir`. In a complete archive it verifies `game-payload.json`, the redistribution notice, and
every file under `game/`, then copies the payload into
`<data root>/runtimes/<assembly-sha256>`. It **checks the selected game build against
`support-matrix.json` and stops if there is no Linux entry**, installs BepInEx `linux_x64`, and
installs the plugin. It creates or applies the map identity and stores the secret in a mode-`0600`
file. Then it writes the start and stop scripts beside itself.

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

**The Windows launcher, which manages up to five worlds from one Windows game folder,
is not part of the Linux kit in this release**, so this rule stands unchanged here: one game
instance per game folder.

If you want a second world on this machine, **unpack a second copy of the game into its own
folder** and install into that. It costs disk and nothing else. `LOCAL-LOGSHRED` in
`error-taxonomy.md` has the shape of it and how to recognise a shredded log.

A complete edition makes that copy for you. Unpack another kit and give it a different
`--data-root` and `--sidecar-port`; the versioned runtime below that data root has its own game
folder and BepInEx log.

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
for other worlds — **and your world's identity**, `peer-secret.txt` beside it, are kept unless you
pass `--remove-world-data`. That is why installing again over the same data root comes back as the
same world on the same slot, and why `--remove-world-data` says on your screen that it is the end
of that world on the map. In a complete install,
unchanged game-payload files are also removed; a changed or user-added file is reported and kept.

There is no `--keep-certificate`, because there is nothing to keep: this installer never wrote to
a trust store.

## If something goes wrong

Four pages are published with this release, and they are written for you rather than for whoever
built it:

- **install** — what the installer does, and what a bare install does.
- **join** — how automatic enrollment and private join strings work, and what joining publishes.
- **diagnose** — what to read, in what order, and what to send when you ask for help.
- **leave** — what stopping, leaving and handing your place over actually mean.

`error-taxonomy.md` beside them lists every refusal this system can hand you, with its remedy
**and who has to act on it** — because the most likely failure on a shared map is one where the
machine that suffers is not the machine at fault.

**Never send anybody a private join string**, in a screenshot, an issue, or a log. No log prints
one, and no diagnostic asks for one.

## What is in this folder

| File | What it is |
|---|---|
| `install-bibites-multiverse.sh` | The installer |
| `uninstall-bibites-multiverse.sh` | The uninstaller |
| `public-map.json` | The public join configuration: HTTPS enrollment and WSS relay addresses; no secret |
| `BibitesMultiverse.dll` | The mod, a BepInEx plugin. **The same file the Windows packages ship** — it is platform-independent IL |
| `multiverse-sidecar` | The program that speaks to the map on your world's behalf. A static binary; it needs no libc of a particular vintage |
| `BepInEx_linux_x64_5.4.23.3.zip` | The mod framework, exactly as its own project publishes it |
| `support-matrix.json` | The game builds this release supports, and the words it refuses with. **The same bytes the Windows packages carry** |
| `LICENSE`, `THIRD_PARTY_NOTICES.md` | The project's Apache-2.0 license and bundled dependency notices |
| `game-payload.json`, `GAME-REDISTRIBUTION-NOTICE.txt`, `game/` | Files that occur only in a complete package |
| `MANIFEST.sha256` | The SHA-256 of every file above, which the installer checks first |
