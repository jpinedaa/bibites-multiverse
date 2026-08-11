# Install

**What you need:** Windows, Steam, and The Bibites. **What you do not need:** a compiler, an
SDK, a runtime, or anything from a developer's toolchain. If an installation step asks you for
one of those, stop — that is a defect, and reporting it is the right response.

> **SLOT — WP6 (the package).** The release page's address, the artifact names, the exact
> commands, and the uninstall. Everything below is the shape of the install and the facts a
> reader has to have before running it; WP6 fills in the specifics from the release as
> published.

## Before you download

**The release page carries published checksums, and it carries them above the download link.**
Check the file you got against the checksum on the page before you run anything. If they
disagree, delete the download and try again; if they disagree twice, report it and do not run
it.

Windows marks every file that came out of a downloaded zip, and it will refuse to run parts of
the package until that mark is cleared. **The release page tells you how**, on the page, before
the download — not in a file you only read after unpacking.

> **SLOT — WP6.** The checksum command and the unblock steps, quoted from the release page so
> the two never drift apart.

## The support matrix, and why the installer may stop

Steam updates The Bibites on its own schedule. This project publishes a **support matrix**: for
each game version, the mod and sidecar build that go with it. **The installer checks your
game's build against that matrix and stops if there is no entry.**

That refusal is doing its job. There is no flag to bypass it, and joining the map with an
unsupported build is not something the software offers. Either wait for a release that lists
your build, or install the build the matrix names.

**The matrix is a per-machine question and never a statement about the map.** Whether your
build can join *this map* is a different test, answered by the relay against the wire version —
see [join.md](join.md).

> **SLOT — WP6 / D22.** Where the matrix is published, and how a reader looks their own build
> up in it.

## What the installer does

It finds Steam's copy of the game, checks the build, installs the mod framework and the
plugin, stores the settings you gave it, and writes the start and stop commands you will use
from then on. **It changes nothing else on your machine**, and the uninstall leaves your game
as it found it.

> **SLOT — WP6.** The installer's own output, including the line where it states the shipped
> export default (below), and the uninstall's steps and its failure modes.

## What a bare install does, stated once

**An unconfigured install exports on all four edges.** Nothing configured means the whole
perimeter, not silence — that is the shipped default and it is deliberate. The installer says
so in its own output, and this document says so here, so that neither is the only place a
reader could have learned it.

**An unconfigured install also connects to nothing.** Without a relay address and a credential
there is no map to join, so the export default is a question about *what your world means to
do* rather than a question about safety. When you join, it means: organisms leave your world on
every side, and arrive from every side.

If you want a wall on one side, say so in your settings. If you want your world off the map
entirely, do not join it.

## The settings a fresh install ships with

Four of them are worth reading before you start, because each one spends something of yours.

| Setting | Ships as | What it costs you |
|---|---|---|
| Export edges | All four | Your world is a full member of the map in every direction. This is the default and it is stated in three places on purpose |
| Migration exclusion | The game's starter species is excluded from export | It keeps founder stock off the lanes. **Setting this to empty turns the policy off**, which floods a shared map with seed genomes and looks entirely normal in the census while it happens |
| Save interval and retained saves | A save every ten minutes, six kept | Six copies of your world on your disk that you did not budget for. The interval is also how often your world pauses to write itself out — see [diagnose.md](diagnose.md) |
| Save on quit | On | Your world is written out when the game closes, so stopping is not losing |

> **SLOT — WP6.** The participant-facing names of these four settings and where they are set,
> which is the packaged install's business rather than the wire's.

## Two flags no document here will ever ask you to pass

The relay and the sidecar each have a flag that disables their authentication for a
single-machine rehearsal. **No installer, script, page or document this project ships may
instruct you to use either one.** If something does, that is the defect.

They are named differently from each other on purpose, so that no instruction can confuse the
two, and neither is a thing a participant needs.

## Next

[join.md](join.md) — what a join string is, what happens on your first claim, and what joining
publishes about your world.
