# The announcement text — copy for the publish step

**This file edits nothing.** It is the text WP3 owes to the documentation slots
other packages left for it, held here so the publish step can consume it in one
pass. `release/` and `docs/` are owned by WP6 and WP7 and are not edited from the
deployment kit.

**Fill three placeholders throughout**, from `deploy.env`:

| Placeholder | From | Example |
|---|---|---|
| `<DOMAIN>` | `MV_DOMAIN` | the owner's registered name |
| `<START>` | `MV_PERIOD_START` | the day the map opens |
| `<END>` | `MV_PERIOD_END` | `<START>` plus three months |
| `<STATUS_PORT>` | `MV_STATUS_PORT` | `8443` |

**The period is three months** (D24, chosen by the owner 2026-08-11). Do not
paraphrase the two rules that come with it: the period is stated **before**
anybody joins, and the bound may be extended by announcement but is **never
silently shortened**.

---

## Slot 1 — `release/RELEASE-PAGE.md`, the `@@OWNER:ANNOUNCED_PERIOD@@` field

The heading above the field already reads *"The map you join is a bounded,
announced commitment"*, so this text starts under it.

```markdown
The public map runs from **<START> to <END>** — **three months**. That is the
whole of the commitment, and it is stated here so that you know it before you
install anything rather than after you have joined.

**Your world is not part of that ending.** It lives on your machine, it saves on
its own schedule, and it goes on being a Bibites world with or without a map to
export to. What ends is the shared map: the relay, its status page, and the
operator's support of them.

**What happens to the record at the end is written down in advance**, not decided
when the date arrives — including what becomes of the map's own files, and what
you are told and when. The reminders begin 30 days out.

**The bound can be extended, and it will never be silently shortened.** An
extension is announced no later than a week before the end; if the run has to
finish early, that too is an announced date at least a week out, with the same
procedure.

Restarts during the run are routine and short. See
[join.md](https://github.com/@@REPO@@/blob/@@TAG@@/docs/participant/join.md) for
what one looks like from your side.
```

## Slot 2 — `docs/participant/join.md`, the slot after "What joining publishes"

Replaces the block beginning *"> **SLOT — WP3 (the hosted deployment).** The
map's announced period, its restart policy..."*.

```markdown
## The map, its period, and its restarts

**The map runs from <START> to <END> — three months.** The period is stated
before anybody joins, it can be extended by announcement, and it will never be
shortened without one. What ends is the shared map; your world is on your machine
and is unaffected.

**Restarts are routine and they are short.** The relay restarts from time to
time: to hand a join string to a new participant, to apply a setting, or to take
a security update. **You do not need to do anything.** Your sidecar notices the
disconnect, waits, reconnects, and re-claims your own slot at your own
coordinate — `reason: "reclaimed"`. Your slot number, your position and your
credential are unchanged; they live on disk, not in the running process.

What you may notice: for a short while after a restart, organisms that were
mid-crossing take longer to arrive. **They are not lost.** They are held by your
own machine and re-sent, and the hold is bounded at 24 hours.

A restart of the **archive** — the service that draws the status page — takes
longer, and the page is unavailable while it happens. The map itself keeps
running the whole time and crossings continue.

Planned restarts are announced. Unplanned ones get an explanation afterwards.

**What happens to the map's records when the run ends** is decided and written
down before the run starts, and the reminders begin 30 days out. Your own
journal, your own saves and your own genomes are yours and stay on your machine
whatever happens to the map's copy.

**One thing the map does delete, and it is stated up front.** The record of every
crossing — who crossed, when, between which worlds, and the ancestry — is kept
for the whole run and beyond. The **genome files** behind it are kept for
**30 days** and then removed from the map's copy. So an organism that crossed
two months ago is still in the record with its family tree; the map may no longer
be able to hand you its genome. Yours is on your own machine, where it always
was.
```

**Also in `join.md`:** three occurrences of `<relay-host>` (the example join
string, the relay URL, and the `multiverse-sidecar --relay` line). Fill each with
`<DOMAIN>`, no port — the relay is on 443, so the URL is
`wss://<DOMAIN>/contract-b/v4`.

## Slot 3 — `docs/participant/leave.md`, "When the map itself ends"

Replaces the block beginning *"> **SLOT — WP3 (the hosted deployment).** The
announced period, the wind-down procedure..."*.

```markdown
**The period is <START> to <END> — three months**, stated before anybody joined.

**How you are told, and when.** Reminders at 30 days, 14 days and 7 days before
the end. The 14-day one says what you can keep — which is everything of yours:
your world, your saves, your journal, your genomes. None of it has to be rescued
from the server. The 7-day one is the last moment an extension can be announced.

**On the day**, the map stops: the relay and the archive are shut down in that
order, and a final capture of the status page — the map as it was at the end, its
species, its flows and its totals — is published with the closing message.

**Within a week after**, the map's own files reach their stated disposition, and a
closing message says what was kept. The credential store is destroyed; it holds
verifiers rather than secrets and is worthless once the relay is gone. The slot
register is kept with the record, because it is what makes the record legible.

**Nothing about your world changes on that day.** It is a Bibites world with no
map to export to, exactly as it was before you joined one.
```

## Slot 4 — `docs/participant/diagnose.md`, "the page's address"

Replaces *"> **SLOT — WP3 (the hosted deployment).** The page's address."*.

```markdown
The map's page is at **https://<DOMAIN>:<STATUS_PORT>/** — note the port; the map
itself is on the standard one and the page has its own.

It is public and read-only. It shows what the relay already broadcasts to every
peer, which is exactly the list in [join.md](join.md) under *What joining
publishes about your world* — nothing more, for you or for anybody else. The
JSON behind it is at `/api/status` if you would rather read it that way, and
`ringstat --url https://<DOMAIN>:<STATUS_PORT>` is the terminal form.
```

## Slot 5 — `docs/error-taxonomy.md` §3.2, `B-4003c`

The open slot asks: *"Where the map publishes the game build it is currently on,
so a refused peer can read it rather than ask."*

```markdown
**Where to read it:** the map's status page, **https://<DOMAIN>:<STATUS_PORT>/**,
publishes every live slot's `gameVersion` — and `/api/status` carries the same
field per slot for a machine reader. A peer refused for an incompatible game
version can therefore read the build the map is on **without asking the
operator**, which is the whole point of the slot: the refusal names a mismatch,
and this is where the other half of the comparison is published.

The page is reachable whether or not your own peer is admitted, and it does not
require a credential.
```

## Slot 6 — `docs/sidecar-diagnose-spec.md`, the `disk-headroom` thresholds

Two rows name WP3 as the owner of *"the multiple of the ceilings that counts as
headroom, and the per-participant growth arithmetic behind it"*. The arithmetic
is now published:

```markdown
**The published growth arithmetic** (WP3, `deploy/SIZING.md`): a participant's
own machine writes its journal, its logs and its genome cache against ceilings it
has already promised itself; the *map's* growth, which is what the operator sizes
a volume from, is **56 MB per day per unit of summed map speed, plus 1.6 MB per
day per slot**. A participant does not need that number to run a world — it is
the hoster's — but it is published so that `disk-headroom`'s `UNKNOWN` has
somewhere to point.
```

---

## The channel copy, for wherever the map is announced

Short form, for a forum post or a message:

```
The Bibites Multiverse public map is open from <START> to <END> — three months.

  Install: <release URL>
  Map:     wss://<DOMAIN>/contract-b/v4   (you get a join string from me)
  Page:    https://<DOMAIN>:<STATUS_PORT>/

Three months is the whole commitment, stated up front. It can be extended by
announcement and it will not be shortened without one. Your world stays on your
machine and is unaffected by any of it.

Joining publishes a fairly complete profile of that world to everybody else on
the map — identity, population, species names, versions, settings, speed and
queue depths. It is written out in full in join.md before you join, not after.

Restarts are routine and short. You do not need to do anything during one.
```

## The reminder templates

**30 days:**

```
The Bibites Multiverse map ends on <END>, 30 days from today. Your world is not
affected — it lives on your machine and keeps running with or without a map.
What ends is the shared map and its page. [State the record's disposition here:
WIND-DOWN.md §4, whichever arm applies.] Two more reminders, at 14 and 7 days.
```

**14 days:**

```
Two weeks left on the Bibites Multiverse map — it ends <END>.

Everything of yours is already yours: your world, your saves, your journal and
your genomes are all on your machine, and nothing has to be rescued from the
server. If you want a copy of what the map recorded about your world, ask before
<END> and I will pull it from the archive.
```

**7 days:**

```
The Bibites Multiverse map ends in a week, on <END>. This is the last reminder
and the last point at which an extension could have been announced; there is no
extension. [Or: the run is extended to <NEW END> — same terms.]

On the day I will publish a final capture of the map — every world, its species
and its flows, as it stood at the end.
```

**The closing message**, after the ending:

```
The Bibites Multiverse map has ended, as announced on <START>. Thank you.

Here is the map as it was at the end: [the four final-*.json captures, or a page
built from them].

What was kept: [WIND-DOWN.md §4 — the arm, and the counts]. The credential store
was destroyed; it held verifiers rather than secrets and had no use once the
relay stopped.

Everything needed to run a map of your own is in the repository — the relay, the
archive and the whole deployment kit at deploy/. It is a small service and it is
not a hard one to host; SIZING.md is the honest arithmetic for what it costs.
```
