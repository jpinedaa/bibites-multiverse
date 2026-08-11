# Leave

**Leaving has three shapes and they are not the same act.** Stopping for a while costs nothing
and needs nobody. Leaving for good needs one message to the operator. Handing your world's
place to somebody else is a third thing again, and it is also the only way to recover a lost
credential.

This page says what each one does to your world, to your slot, and to the organisms in flight.

## 1. Stopping for a while

**Just stop.** Close the game, stop the sidecar, turn the machine off. Nothing has to be told.

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

| What | What happens |
|---|---|
| Your position | Becomes an ordinary **hole** in the map — the next newcomer fills it before any axis grows |
| Your slot number | **Retired forever.** Slot numbers are never reused, and that is what makes the next line safe |
| Organisms still addressed to your slot | Get a **permanent** answer — that world never returns, so no retry can ever succeed. One the relay can prove it never handed anywhere goes home at once; one that may already have reached your sidecar before you went waits out its hold first, because the point of the wait is exactly that possibility |
| Your neighbours | Their lanes re-pair around the hole. No slot number changes and no other position moves |
| Your world | Yours. The saves are on your disk, and the uninstall leaves your game as it found it |
| Your credential | Stops working. There is nothing to revoke on your side |

**Before you go, drain your journal.** Your sidecar may be holding organisms it took custody of
and has not been able to hand on — held entries, waiting on a destination that is dark. Custody
is local: **nobody else can do this for you, and no operator command can reach it.** List them,
then release each one, choosing whether it goes home to the world it came from or is dropped.
The release prints its consequences before it acts, including the one case where a bounce can
produce a duplicate.

> **SLOT — WP7, later arc.** The packaged names of the list and release commands, and the exact
> wording of the duplication warning.

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
| Timing | The relay **refuses a handover while the old world is still connected**, and says so. A live world with its place taken from under it would keep claiming, keep being refused, and keep running with nowhere to export |

**This is the only credential recovery there is**, and it is deliberate: the alternative is an
account system with an email and a reset flow, and the map is a hobby project that chose not to
have one. The cost was accepted with the price in view.

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

> **SLOT — WP3 (the hosted deployment).** The announced period, the wind-down procedure, and
> what a participant is told and when.
