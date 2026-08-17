# Diagnose

**Four things to do, in this order.** Each one answers a different question, and doing them out
of order is how an evening gets spent on the wrong machine.

1. **Read what the map thinks of your world**, with `multiverse-sidecar --my-slot`.
2. **Run `multiverse-sidecar --diagnose`.** It runs the checks a person used to have to run.
3. **Look the symptom up in the taxonomy.** Every refusal there names its remedy *and* who has
   to apply it.
4. **Ask, with the five things a question needs.**

**Both commands are the sidecar you already have**, run from the folder you installed from, and
both need to be told which world they are about:

```powershell
cd <the folder you unpacked the release into>
$data = "$env:LOCALAPPDATA\BibitesMultiverse\data"     # the world the installer made

.\multiverse-sidecar.exe --my-slot   --data-dir $data
.\multiverse-sidecar.exe --diagnose  --data-dir $data
```

```sh
cd <the folder you unpacked the release into>
data="${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse/data"   # unless you passed --data-root

./multiverse-sidecar --my-slot   --data-dir "$data"
./multiverse-sidecar --diagnose  --data-dir "$data"
```

**`--data-dir` names ONE world, and on Windows you may have more than one.** Every world the
launcher created has its own data folder, so the path above is only the world the installer made —
run either command against another world's folder and you get a clean report for a world that is
not the one failing. `multiverse-launcher.exe status --all` names every world on this
computer with its data folder, and `multiverse-launcher.exe profile show NAME` shows one of
them. The `data` directory these commands want is `<that world's data root>\data`. The path above
is the whole story on Linux, where a second world means a second unpacked kit with its own
`--data-root`.

**`--diagnose` reports on the configuration you hand it, so hand it your map.** With `--data-dir`
alone the sidecar uses its own default relay — a local address — and the two checks that are about
your world's place on the map, `relay-reachable` and `credential`, then answer about a relay your
world has nothing to do with. Add the relay your world dials and the file its secret lives in:

```powershell
$root = Split-Path $data -Parent
.\multiverse-sidecar.exe --diagnose --data-dir $data `
    --relay (Get-Content "$data\relay-url") --credential-file "$root\peer-secret.txt"
```

```sh
root="$(dirname "$data")"
./multiverse-sidecar --diagnose --data-dir "$data" \
    --relay "$(head -n 1 "$data/relay-url")" --credential-file "$root/peer-secret.txt"
```

**On Windows the launcher does this for you**: **Run a health check** in the launcher's window runs
the diagnostic with that world's own relay and credential filled in, and prints the command line it
used above the report. The window's details pane opens by itself to show it. `--credential-file` names the file and never the secret; no flag in this system
takes a secret as a value.

Everything below writes them short. **The flags are the same on both platforms**; what differs is
the file name and where your data directory is. **They read; they do not start anything**, and you
can run either one while your world is running.

## 1. Read what the map thinks of your world

```
multiverse-sidecar --my-slot
```

**Your own sidecar answers this, not the map.** It already knows everything the map has said
about your world — it was told, on the same broadcast every other peer gets — so this asks the
map for nothing, works when the map's own page is unreachable, and is about you rather than about
a grid of six worlds. Add `--json` for the machine-readable form.

The map also publishes a page with every world on it, including yours, and it is worth reading
when the question is about somebody else's world rather than your own.

The project website is at **https://bibitesmultiverse.com/**. The live console is at
**https://bibitesmultiverse.com/live**.

The console is public and read-only. It shows the same world information that the relay sends
to every peer. [join.md](join.md) lists this information under *What joining publishes about
your world*. The machine-readable form is at `/api/status`. The terminal form is
`ringstat --url https://bibitesmultiverse.com`.

What to read, and what a healthy reading looks like:

| What | Healthy | What a bad reading means |
|---|---|---|
| Your slot is **live**, with a game connected | both true | Live with no game connected is the worst-looking symptom in this system and it has two very different causes — see `LOCAL-CONFIGRACE` and `LOCAL-STARVATION` in the taxonomy. **On Linux the second of those does not happen**; what happens instead is `LOCAL-LOGSHRED`, where every world works and the log is the casualty |
| Your **edges** | Every edge you declared reports a live peer | Each closed reason means something different, and only some of them are yours to fix — taxonomy §4 |
| Your **queue depths** — in custody, paced, unresolved | Small, and moving, with nothing lost | A paced depth that never falls names a delivery rate set too low. Read it against your own configured rate, which is published beside it. An **unresolved** entry is an organism handed to the map once and not answered for yet: it is not re-sent and it does not come home, so if no answer arrives it is recorded **lost** and counted on a line of its own. **Late answers** on that line are good news — the organism did arrive, and the count is telling you `--forward-timeout` is set shorter than this map's slowest honest answer |
| Your **last save** | Recent, against your own save interval | An absent save with a save interval of zero is a world with its save timer off — a reading, not a gap |
| Your **speed** | The applied speed and the achieved speed are close | When they come apart, your machine cannot meet the speed you asked for, and **the gap is the news**. Every rate about your world is wrong by that factor |
| Your **last refusal** | Absent | Present means the relay refused this world for a stated reason, and it is published to every peer rather than only logged, so that a stale world does not read as a dead one. It names one of three things: an incompatible game version, a wire version below the map's floor, or `capacity:` and the limit that fired. **Two refusals deliberately never appear here** — a rejected credential, which reaches no slot at all, and an eviction, which has no shape of its own. An empty field is not proof that nothing was refused |
| The **other worlds** | Live, with a game connected, on your build | This is the row nobody could read before. When your lanes are quiet and your own world looks healthy, the cause is usually here, and it is on somebody else's machine |

**`--my-slot` shows and does not judge.** It prints the readings; `--diagnose` is what says
whether one of them is a fault, and who has to act. If your sidecar is not running, `--my-slot`
has nothing to read and says so — run `--diagnose`, which answers a dozen questions without it.

## 2. Run `--diagnose`

```
multiverse-sidecar --diagnose
```

It runs, on your machine, the checks that were done by hand for the whole of this project's
development: your data directory, the relay's reachability, its certificate, your credential,
the wire version against the map's floor, your slot, your game's connection, your export set
against the map's shape, your lanes, your world's speed, your journal, your save health, your
disk headroom, and your neighbours' versions.

**Four things it will not do**, and they are guarantees rather than limitations:

- **It changes nothing.** It never rotates a credential, restarts a game, sets a speed, claims a
  slot or writes to your journal. It makes exactly one write anywhere: an empty file in your data
  directory, removed at once, because whether a directory can be written to is not a thing you
  can find out by looking at it.
- **It never prints a secret**, in whole or in prefix. It prints the *path* each one is read
  from, which is the fact you can act on. **The whole report is safe to send.**
- **It runs without the map.** With no relay to reach, the checks that need one report
  *unknown* and say which check they were waiting on — they do not fail, and they do not
  quietly pass.
- **It runs beside your sidecar without disturbing it.** It reads your running sidecar's own
  state rather than opening a second connection to the map, and it never asks you to stop
  anything.

**Each line is one check**: the verdict, the check's name, and one sentence. Under anything that
is not a pass come the evidence, the remedy, the taxonomy id and **who must act**. It ends with
the counts and what its exit code means.

| It exits | When |
|---|---|
| `0` | Nothing failed. Warnings and unknowns may be present, and on a healthy machine several always are |
| `1` | Something failed |
| `2` | The command itself could not run — no data directory, or `--check` naming something that is not a check. It has told you nothing |

**An unknown is not a failure and not a pass.** It is a question nobody could answer yet, and it
names what it was waiting on.

Useful arguments: `--json` for the machine-readable form to paste into a support conversation,
`--check <name>,<name>` to report only some of them, and `--timeout` to bound each probe on a slow
link. **On a packaged install it needs nothing else**: your game folder and the support matrix
come from the `install-record.json` the installer left in your data root, which is also how it
knows where to look for the mod's log. **A world the launcher created has no install record of its
own** — it shares the game folder the installer bound, so point `--data-dir` at that world and, if
a check asks for the game, give it the same game folder `profile show NAME` prints. The specification it is built from is
[`../sidecar-diagnose-spec.md`](../sidecar-diagnose-spec.md), which lists every check, its pass
criterion and the taxonomy entry its failure points at.

## 3. Look it up

[`../error-taxonomy.md`](../error-taxonomy.md) is the whole list. It is organised by **where
you saw the symptom**, because that is what you know first:

| You saw it… | Read |
|---|---|
| While installing | §1 |
| Between your game and your sidecar, on your own machine | §2. **Nothing in that section can be caused by another participant** |
| Between your sidecar and the map | §3 |
| As a closed edge | §4 |
| As nothing at all — a world that is running and not participating | §5 |
| As something the operator evidently did | §6 |

**The one thing worth internalising before you start**: this system's most likely public
failure is one where the machine that suffers is not the machine at fault. A world whose lanes
have gone quiet may be perfectly healthy and standing next to one that is not. That is why
every entry names an actor, and why *operator* is one of the three — not because the operator
broke it, but because they are the only party who can see both ends.

## 4. Ask, with five things

1. **The taxonomy id**, if you found one.
2. **Four versions**: game, mod, sidecar, and the wire versions each side reported.
3. **The close code and its reason string**, verbatim. Nothing parses a reason string; it is
   written for a person, and it is where the map explains itself.
4. **The log lines either side of it.** A packaged install writes two logs per world — three on
   Windows, with the launcher's own — and the game's is **truncated on every launch**, so copy it
   before you restart the game:

   | | Windows | Linux |
   |---|---|---|
   | The game and the mod | `<game folder>\BepInEx\LogOutput.log`, rotating to `.1`–`.4` when more than one world runs from that folder | `<game folder>/BepInEx/LogOutput.log` |
   | The sidecar | `<that world's data root>\logs\sidecar.log` — `%LOCALAPPDATA%\BibitesMultiverse\logs\sidecar.log` for the world the installer made | `${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse/logs/sidecar.log`, with `logs/game.out` beside it |
   | The launcher **(Windows)** | `<that world's data root>\logs\launcher.log`, one line per event | not in this release; the Linux kit ships no launcher |

   Say which world you are sending, when this computer runs more than one, and send that world's
   logs rather than the installer's. Menu item **8) Open this world's log folder** opens the right
   one. None of these ever contains your credential. **On Linux, check the mod log is still a log before you
   send it:**

   ```sh
   file "<game folder>/BepInEx/LogOutput.log"
   ```

   If that answers `data` rather than text, more than one game instance has run out of that game
   folder and the log has been shredded — see `LOCAL-LOGSHRED`. Say so when you send it; what it
   contains is not what happened.
5. **Your world's identity and slot.** Both are public. **Say which platform you are on**, too:
   several install refusals and local failures exist on one platform only.

**Never send a credential or a token.** A message that contains one turns a support question
into a slot handover.

## What the operator can and cannot see

Knowing the boundary saves a round trip in both directions.

**They can see**, for every world at once: liveness, slot and position, population, census,
mod and wire and game versions, save policy, exclusion list, speed, queue depths, **how many
organisms each world has lost in transit**, and every refusal that reached a slot. They can also
see the one thing you cannot — **your neighbours' side of a shared failure**.

**They cannot see inside your world or your journal.** Custody is local by design: the
organisms your sidecar holds live in a file on your machine, and there is no protocol that
moves them and no operator command that reads them. When the question is *which* organisms are
held and what they are, the operator's honest answer is **"ask the peer"**, and the peer is
you. Two commands answer it, on your machine and nowhere else, and **both need your sidecar
stopped** because the journal takes one writer:

```
multiverse-sidecar --list-inflight
multiverse-sidecar --release-inflight <migrationId> bounce|drop
```

Written out for your platform in [leave.md](leave.md); the flags are the same on both. Point
`--data-dir` at the data root of the world you are asking about — on Windows,
`multiverse-launcher.exe status --all` names each one.

The list says, for each organism handed over, when it went and how long is left before it is
written off as lost. The release prints the entry, then the duplication risk, and waits for a
typed `YES` **before it acts**. [leave.md](leave.md) gives both commands in full, and
[`../error-taxonomy.md`](../error-taxonomy.md) §2.4 gives the warning's exact words and why
bouncing an entry the far side may already hold is now **the only way left to duplicate an
organism on this map** — nothing here does it on its own any more.

## Next

[leave.md](leave.md) — what stopping, leaving and handing over actually mean, and what becomes
of your world either way.
