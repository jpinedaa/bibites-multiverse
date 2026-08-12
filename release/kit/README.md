# Bibites Multiverse — install kit

**This folder joins one Bibites world to a shared map.** Organisms leave your world through its
edges and arrive in other people's; theirs arrive in yours. Your world stays on your machine and
stays yours.

**You need:** Windows, Steam, and The Bibites. **You do not need:** a compiler, an SDK, a
runtime, or anything from a developer's toolchain. Nothing here will ever ask you to turn a
security control off — no execution-policy bypass, no `--insecure` flag, no skipped certificate
check. If any part of this package asks you for one of those, that is a defect and reporting it
is the right response.

You also need **a join string**, which the operator of a map hands you out of band. It looks
like this, on one line:

```
multiverse-join/1 wss://<relay-host>/contract-b/v4 <your-world>.<secret>
```

## Before you run anything

The release page you downloaded this from carries the archive's SHA-256 **above** the download
link, and the mark-of-the-web steps beside it. Do those two things there, in that order, if you
have not already: **check the archive against the published checksum, then clear the mark on the
archive before you unpack it.**

The installer checks itself again anyway. Its first step verifies every file in this folder
against `MANIFEST.sha256` and refuses to go on if one of them disagrees.

## Install

```powershell
.\Install-BibitesMultiverse.ps1
```

It asks for your join string with the typing hidden. **There is no parameter that takes the
secret on the command line**, on purpose: a value typed on a command line is in every process
listing on this machine and in your shell history. If you would rather not type it, put the
one-line join string in a file and pass `-JoinStringFile .\join.txt` — then delete that file.

The installer finds Steam's copy of The Bibites, **checks your game build against
`support-matrix.json` and stops if there is no entry for it**, installs BepInEx and the plugin,
stores the secret half of your join string in a file only you can read, states the settings it
is giving you, and writes `Start-Multiverse.ps1` and `Stop-Multiverse.ps1` beside itself.

It imports nothing into any trust store. `-CaFile` exists for a private or LAN map whose relay
signs its own certificate, and only then.

**What a bare install does:** your world **exports on all four edges**. Nothing configured means
the whole perimeter, not silence. Every edge is a door that works both ways. The installer says
so on your screen while it runs, and `docs/participant/install.md` says so on the page.

## Start and stop

```powershell
.\Start-Multiverse.ps1      # the sidecar, then the game
.\Stop-Multiverse.ps1       # the game, then the sidecar
```

`Start-Multiverse.ps1 -Headless` runs the world with nothing drawn; the simulation is unchanged.
`Stop-Multiverse.ps1 -GameOnly` puts the world down and leaves the sidecar up, so it keeps your
place on the map and keeps taking custody of everything that arrives while you are away.

**Stopping costs nothing and needs nobody.** Your place on the map is keyed on your world's
identity and never expires.

## The settings this install ships with

All five are written explicitly into `Start-Multiverse.ps1`, including the ones that match the
mod's own default, so that a future change to a default cannot silently move what your world
does. Edit them there.

| In `Start-Multiverse.ps1` | Ships as | What it spends |
|---|---|---|
| `MULTIVERSE_EXPORT_EDGES` | `E,N,W,S` | Your world is a full member of the map in every direction |
| `MULTIVERSE_MIGRATION_EXCLUDE` | `Basic bibite` | Keeps the game's starter species off a shared map's lanes. An empty value turns the policy off, and the installer makes you say `-NoMigrationExclusion` to mean it |
| `MULTIVERSE_SAVE_MINUTES` | `10` | How often your world pauses to write itself out |
| `MULTIVERSE_SAVE_KEEP` | `6` | Six copies of your world on your disk |
| `MULTIVERSE_SAVE_ON_QUIT` | `true` | Your world is written out when the game closes |

## Uninstall

```powershell
.\Uninstall-BibitesMultiverse.ps1 -DryRun   # the ledger, changing nothing
.\Uninstall-BibitesMultiverse.ps1
```

It reads the record the installer wrote and removes **only** what is named in it, hash-checked,
printing a line per path for what it removed and what it kept. Your worlds and their backups are
never touched. Your journal — the organisms this machine is holding for other worlds — is kept
unless you pass `-RemoveWorldData`.

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
| `Install-BibitesMultiverse.ps1` | The installer |
| `Uninstall-BibitesMultiverse.ps1` | The uninstaller |
| `BibitesMultiverse.dll` | The mod, a BepInEx plugin |
| `multiverse-sidecar.exe` | The program that speaks to the map on your world's behalf |
| `BepInEx_win_x64_5.4.23.3.zip` | The mod framework, exactly as its own project publishes it |
| `support-matrix.json` | The game builds this release supports, and the words it refuses with |
| `MANIFEST.sha256` | The SHA-256 of every file above, which the installer checks first |
