# Diagnose

**Four things to do, in this order.** Each one answers a different question, and doing them out
of order is how an evening gets spent on the wrong machine.

1. **Read what the map thinks of your world.** It is one page, and it is about you.
2. **Run `multiverse-sidecar --diagnose`.** It runs the checks a person used to have to run.
3. **Look the symptom up in the taxonomy.** Every refusal there names its remedy *and* who has
   to apply it.
4. **Ask, with the five things a question needs.**

## 1. Read what the map thinks of your world

The map publishes a page, and your world is on it beside everybody else's. You do not need an
operator to read it to you.

What to read, and what a healthy reading looks like:

| What | Healthy | What a bad reading means |
|---|---|---|
| Your slot is **live**, with a game connected | both true | Live with no game connected is the worst-looking symptom in this system and it has two very different causes — see `LOCAL-CONFIGRACE` and `LOCAL-STARVATION` in the taxonomy |
| Your **edges** | Every edge you declared reports a live peer | Each closed reason means something different, and only some of them are yours to fix — taxonomy §4 |
| Your **queue depths** — in custody, paced, held | Small, and moving | A paced depth that never falls names a delivery rate set too low. Read it against your own configured rate, which is published beside it |
| Your **last save** | Recent, against your own save interval | An absent save with a save interval of zero is a world with its save timer off — a reading, not a gap |
| Your **speed** | The applied speed and the achieved speed are close | When they come apart, your machine cannot meet the speed you asked for, and **the gap is the news**. Every rate about your world is wrong by that factor |
| Your **last refusal** | Absent | Present means the relay refused this world for a stated reason, and it is on the page rather than only in a log so that a stale world does not read as a dead one |

> **SLOT — WP3 (the hosted deployment).** The page's address.
>
> **SLOT — WP7, later arc.** The participant's own view of their slot. Every field above
> already exists on the wire; presenting them as *your world's* answer rather than as one cell
> in an operator's grid is the work.

## 2. Run `--diagnose`

```
multiverse-sidecar --diagnose
```

It runs, on your machine, the checks that were done by hand for the whole of this project's
development: your data directory, the relay's reachability, its certificate, your credential,
the wire version against the map's floor, your slot, your game's connection, your export set
against the map's shape, your lanes, your world's speed, your journal, your save health, your
disk headroom, and your neighbours' versions.

**Three things it will not do**, and they are guarantees rather than limitations:

- **It changes nothing.** It never rotates a credential, restarts a game, sets a speed or
  writes to your journal.
- **It never prints a secret**, in whole or in prefix.
- **It runs without the map.** With no relay to reach, the checks that need one report
  *unknown* and say which check they were waiting on — they do not fail, and they do not
  quietly pass.

Every failure it reports names a taxonomy id and who must act. The specification it is built
from is [`../sidecar-diagnose-spec.md`](../sidecar-diagnose-spec.md), which lists every check
and its pass criterion.

> **SLOT — WP7, later arc.** The command's exact output, its exit codes and its machine-readable
> form. The spec proposes all three; the implementation fixes them.

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
4. **The log lines either side of it.**
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
you. The commands that list and release held entries run on your machine and nowhere else.

> **SLOT — WP7, later arc.** Those commands' packaged names and the duplication warning the
> release prints before it acts.

## Next

[leave.md](leave.md) — what stopping, leaving and handing over actually mean, and what becomes
of your world either way.
