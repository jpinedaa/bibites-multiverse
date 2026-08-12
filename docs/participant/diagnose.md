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
$data = "$env:LOCALAPPDATA\BibitesMultiverse\data"     # unless you passed -DataRoot

.\multiverse-sidecar.exe --my-slot   --data-dir $data
.\multiverse-sidecar.exe --diagnose  --data-dir $data
```

Everything below writes them short. **They read; they do not start anything**, and you can run
either one while your world is running.

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

> **SLOT — WP3 (the hosted deployment).** The page's address.

What to read, and what a healthy reading looks like:

| What | Healthy | What a bad reading means |
|---|---|---|
| Your slot is **live**, with a game connected | both true | Live with no game connected is the worst-looking symptom in this system and it has two very different causes — see `LOCAL-CONFIGRACE` and `LOCAL-STARVATION` in the taxonomy |
| Your **edges** | Every edge you declared reports a live peer | Each closed reason means something different, and only some of them are yours to fix — taxonomy §4 |
| Your **queue depths** — in custody, paced, held | Small, and moving | A paced depth that never falls names a delivery rate set too low. Read it against your own configured rate, which is published beside it |
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
knows where to look for the mod's log. The specification it is built from is
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
4. **The log lines either side of it.** A packaged install writes exactly two logs: the game and
   the mod to `<game folder>\BepInEx\LogOutput.log`, which is **truncated on every launch**, so
   copy it before you restart the game; and the sidecar to
   `%LOCALAPPDATA%\BibitesMultiverse\logs\sidecar.log`. Neither ever contains your credential.
5. **Your world's identity and slot.** Both are public.

**Never send a credential or a token.** A message that contains one turns a support question
into a slot handover.

## What the operator can and cannot see

Knowing the boundary saves a round trip in both directions.

**They can see**, for every world at once: liveness, slot and position, population, census,
mod and wire and game versions, save policy, exclusion list, speed, queue depths, and every
refusal that reached a slot. They can also see the one thing you cannot — **your neighbours'
side of a shared failure**.

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

The release prints the entry, then the duplication risk, and waits for a typed `YES` **before it
acts**. [leave.md](leave.md) gives both commands in full, and
[`../error-taxonomy.md`](../error-taxonomy.md) §2.4 gives the warning's exact words and why
bouncing an entry the far side may already hold is the one exception at-most-once carries.

## Next

[leave.md](leave.md) — what stopping, leaving and handing over actually mean, and what becomes
of your world either way.
