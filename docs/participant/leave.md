# Leave

**Leaving has three shapes and they are not the same act.** Stopping for a while costs nothing
and needs nobody. Leaving for good needs one message to the operator. Handing your world's
place to somebody else is a third thing again, and it is also the only way to recover a lost
credential.

This page says what each one does to your world, to your slot, and to the organisms in flight.

## 1. Stopping for a while

**Just stop.** Close the game, stop the sidecar, turn the machine off. Nothing has to be told.

**One thing worth doing rather than just pulling the plug**: let the game finish quitting, so
save-on-quit runs. On Windows, the launcher's **Stop this world** — or
`BibitesMultiverseLauncher.exe stop`, or `stop --all` for every world on the computer — asks the
game to close and waits up to thirty seconds for that save before it insists. `Stop-Multiverse.ps1`
does the same, and `stop-multiverse.sh` waits up to twenty seconds on Linux. A clean quit with its
save has measured at about two seconds. **A headless world on Windows is the exception**: it has no
window to close, so it is stopped outright and can lose the time since its last save. That is
`LOCAL-HEADLESSSTOP` in [`../error-taxonomy.md`](../error-taxonomy.md), which gives the two ways
around it. A world
killed outright loses everything since its last save, which is a loss of *your* progress and never
of anybody's organisms: custody lives in the journal, not in the world file.

| What | What happens |
|---|---|
| Your slot and position | **Yours.** The reservation is keyed on your world's identity, never on a connection, and **it never expires** |
| Coming back | You reclaim the same slot at the same position. Your neighbours re-pair to you as an ordinary liveness event — nothing is re-inserted and no lane moves |
| Your neighbours, meanwhile | They route around you. A lane whose next world is dark walks on to the next live one along the same axis, so the map keeps working with a hole in it |
| Organisms addressed to you | Their senders hold them and keep retrying. **The hold is bounded**: after 24 hours of *accrued* dark time, the organism goes home to the world it left, and the bounce is reported rather than silent |
| The clock on that hold | It runs only while you are dark **and** the sender can see the map. A sender whose own machine slept for a night does not wake with an expired clock — it never observed you dark, it only failed to observe anything |
| Your own organisms in flight | In your journal, on your disk. They are still yours, and they re-route or release when you come back |
| Your world | On your disk. Your saves are your saves |

**If you are going to be away for longer than a day**, the only consequence is that some
organisms addressed to you come home to their own worlds instead of arriving in yours. Nothing
is lost and nothing needs your attention.

## 2. Leaving for good

**Tell the operator.** That is the whole of your part. They release your slot, and the release
is what makes the map tidy rather than permanently expectant.

**One message per world.** Each world you run is a separate identity with its own slot, so if this
computer runs several, name the ones you are leaving with.

| What | What happens |
|---|---|
| Your position | Becomes an ordinary **hole** in the map — the next newcomer fills it before any axis grows |
| Your slot number | **Retired forever.** Slot numbers are never reused, and that is what makes the next line safe |
| Organisms still addressed to your slot | Get a **permanent** answer — that world never returns, so no retry can ever succeed. One the relay can prove it never handed anywhere goes home at once; one that may already have reached your sidecar before you went waits out its hold first, because the point of the wait is exactly that possibility |
| Your neighbours | Their lanes re-pair around the hole. No slot number changes and no other position moves |
| Your world | Yours. The saves are on your disk, and the uninstall — `Uninstall-BibitesMultiverse.ps1`, or `./uninstall-bibites-multiverse.sh` — leaves your game as it found it, which a test proves against a sandbox game tree rather than a page asserting it. See [install.md](install.md). It keeps your journal **and your world's identity** — `peer-secret.txt`, and `data/peer-id` beside the journal — unless you pass `-RemoveWorldData` / `--remove-world-data`, and it never goes near your worlds. That is why installing again over the same data root comes back as the **same** world on the **same** slot, and why removing the software is not leaving the map |
| Removing one world of several | `BibitesMultiverseLauncher.exe profile delete NAME` removes that world from this computer. **It is not leaving the map**: the slot stays reserved for that identity until the operator releases it, so send the message as well. It keeps that world's journal, logs and credential unless you pass `--remove-world-data`, which ends that world on the map: the relay keeps a verifier and cannot mint the secret again. Either way it asks you to type the world's name, and with `--remove-world-data` it asks **even under the global `--yes`** |
| Your credential | **Still authenticates until the operator drops it** — a release retires a *reservation*, not an identity. What is gone is your place: connect again and the map treats you as a newcomer, at a new slot number and wherever the ordinary placement rules put you. Your old slot number is never reused. There is nothing to revoke on your side |

**Before you go, drain your journal.** Your sidecar may be holding organisms it took custody of
and has not been able to hand on — held entries, waiting on a destination that is dark. Custody
is local: **nobody else can do this for you, and no operator command can reach it.**

**Stop your sidecar first** — `BibitesMultiverseLauncher.exe stop` or `.\Stop-Multiverse.ps1` on
Windows, `./stop-multiverse.sh` on Linux —
because the journal takes one writer and both commands refuse while it is running. Then list what
is left, and release each entry, choosing whether it goes home to the world it came from or is
dropped:

```powershell
cd <the folder you unpacked the release into>
$data = "$env:LOCALAPPDATA\BibitesMultiverse\data"     # unless you passed -DataRoot

.\multiverse-sidecar.exe --list-inflight    --data-dir $data
.\multiverse-sidecar.exe --release-inflight <migrationId> bounce|drop --data-dir $data
```

```sh
cd <the folder you unpacked the release into>
data="${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse/data"   # unless you passed --data-root

./multiverse-sidecar --list-inflight    --data-dir "$data"
./multiverse-sidecar --release-inflight <migrationId> bounce|drop --data-dir "$data"
```

For a world the launcher created, the data folder is that world's own.
`BibitesMultiverseLauncher.exe status --all` names it for every world on this computer.

The list gives you the migration id, the organism, where it was going, and how long its hold
clock has accrued. The release prints the entry and then **the duplication risk, before it acts**,
and waits for you to type `YES`: an entry that was already written to a live relay connection may
already be held by the far side, and bouncing that one home can leave the map holding two copies.
An entry that was never handed to anybody cannot duplicate. A `drop` is a loss you chose, and it
says so. The exact wording is in [`../error-taxonomy.md`](../error-taxonomy.md) §2.4.

**If you simply stop and never say anything**, nothing breaks — but the map keeps a place for
you indefinitely. Your neighbours route around you forever, your position is never filled by
anybody else, and every organism addressed to your slot waits out its full hold before going
home. Releasing a slot is the operator's answer for a position that will never be filled again,
and one message from you is what tells them it is that kind of absence.

## 3. Handing your world's place to somebody else

**Or getting your own place back after losing your join string.** The same act does both: the
operator rebinds your reservation — slot number *and* position — to a new identity with a
freshly minted credential.

| What | What happens |
|---|---|
| The map's shape | Unchanged. No lane moves and no slot number changes |
| The new occupant | **Inherits nothing.** An empty journal and a different world. They are not told about the old world's organisms, because there is nothing correct they could do with them |
| Work in flight addressed to that slot | **Arrives at whoever is there now**, because routing is on the position and not on a person. A handover rebinds a place in the map, and an organism travelling to that place arrives at its current occupant. An operator who does not want that outcome wants a release instead |
| The old world's journal | Stays with the old world. A released or replaced world keeps two obligations: it delivers its own inbound organisms to its own game even from outside the map, and it re-routes or releases its outbound ones if it ever rejoins |
| The two credentials | The act **mints the new identity's credential and drops the old identity's**, in one step. The new join string is printed once, in the answer the operator reads when they confirm — the single moment that secret exists anywhere but its owner's machine — and from that moment the old one authenticates nothing |
| Timing | The relay **refuses a handover while the old world is still connected**, and says so. A live world with its place taken from under it would keep claiming, keep being refused, and keep running with nowhere to export |
| How it is done | Two calls on the operator's side: one that returns the consequences — the slot, its position, its current identity, how long it has been dark, whose lanes change — and a second that applies them, refused if the map moved in between. **Nothing a participant sends can invoke one**; it is a separate authenticated path, not this wire |

**This is the only credential recovery there is**, and it is deliberate: the alternative is an
account system with an email and a reset flow, and the map is a hobby project that chose not to
have one. The cost was accepted with the price in view.

**What the installer does when it meets a lost secret.** A data root that still names a world —
`data/peer-id` beside its journal — but has no `peer-secret.txt` stops the installer, which prints
that world's identity. It will not quietly take a new one, because a new identity without this
handover is a second place on the map and leaves the old one dark. Ask for the handover above and
install the join string it prints with `-JoinStringFile` **and** `-ReplaceWorldIdentity` /
`--replace-world-identity` — the switch is what tells the installer that this folder's world is
meant to change identity, and it is needed for the same reason whether the new identity arrived
from a handover or from nowhere. Either way the old name is kept in `data/peer-id.previous`, which
is the string this operator conversation needs.

## What you cannot take with you

**The record.** Every crossing your world made is in the map's archive, and nothing evicts from
it — that is what makes it a record. If a name from your world needs to stop being shown, the
operator can suppress it **at the view**: it disappears from the map's page and its terminal
view, your world is untouched, and nothing is asked of your machine. **What cannot be promised
is removal from the record.** That distinction is deliberate and it is stated here rather than
discovered during a support conversation.

## When the map itself ends

**The map is a bounded, announced commitment**, not an open-ended service. The period is stated
before anybody joins, and the ending is a stated event with a wind-down that says what happens
to the map's own durable files — the slot-to-identity register and the archive's record.

Your world is not part of that ending. It is on your machine, it saves on its own schedule, and
it goes on being a Bibites world with or without a map to export to.

**The period is 2026-08-14 to 2026-11-14 — three months**, stated before anybody joined.

**How you are told, and when.** Reminders arrive 30, 14, and 7 days before the end. The
14-day reminder lists what you keep: your world, saves, journal, and genomes. You do not need
to rescue them from the server. The 7-day reminder is the last time an extension can be
announced.

**On the final day**, the relay stops first, followed by the archive. The operator publishes a
final status-page capture with the closing message. It shows the map, species, flows, and totals
at the end.

**Within one week**, the map's files reach their stated disposition. A closing message says
what was kept. The credential store is destroyed because it has no use after the relay stops.
The slot register stays with the record so the record remains legible.

**Nothing about your world changes on that day.** It becomes a Bibites world without a map,
just as it was before you joined.
