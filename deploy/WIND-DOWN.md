# Wind-down — the ending, as a stated event

**D24 made the hosted relay a bounded, announced commitment.** DQ2 turned that
into an operational obligation rather than only a social one:

> *"The period is stated before anybody joins; what a restart looks like is
> stated while it runs; and the **ending** has to say what becomes of `ring.json`
> and the archive's three durable files, which is where decision 3's retention
> rule is actually consumed. A service with an announced end needs a wind-down
> procedure in the same way it needs a restart policy, and it is the same kind of
> document — written before the participants arrive, not after they ask."*

This is that document, written before they arrive.

**The period: three months.** Chosen by the owner 2026-08-11. Start and end dates
live in `deploy.env` as `MV_PERIOD_START` and `MV_PERIOD_END`; every `<START>`
and `<END>` below is filled from them.

**The rule that governs the rest of this file: the bound may be EXTENDED by
announcement. It is never silently shortened.** §5.

---

## 1. What ends, and what does not

**Ends:** the hosted relay, the hosted archive, the status page, the domain
pointing at them, and the operator's support of any of it.

**Does not end:** anybody's world. A participant's Bibites world is on their own
machine, it saves on its own schedule, and it goes on being a Bibites world with
or without a map to export to. `docs/participant/leave.md` already says so and it
is worth repeating in every message below, because it is the sentence that turns
an ending from a loss into a stop.

**Also does not end:** the ability to run a map. D24 asked for the
publish-the-relay path to be *prepared* so that the answer to "what happens
after" is not improvised in front of participants. §6.

## 2. The timeline

Each row is an act, not an intention. The dates are relative to `<END>`.

| When | Act | Where |
|---|---|---|
| **Before the first join** | The period is stated: the release page, `docs/participant/join.md`, `docs/participant/leave.md`. Nobody joins without knowing when it ends | `ANNOUNCEMENT.md` |
| **`<END>` − 30 days** | First reminder. States the date, restates that their world is theirs, and states which arm of §4 applies to the record | the map's channel |
| **`<END>` − 14 days** | Second reminder, plus **what a participant can keep**: their own journal, their own save files, their own genomes. Nothing has to be rescued from the server for a participant's world to survive | the map's channel |
| **`<END>` − 7 days** | Final reminder. If the run is being extended, the extension is announced **here at the latest**, never after the end | the map's channel |
| **`<END>` − 1 day** | Operator: a Lightsail snapshot by hand, and `backup.sh --full`. Read the final numbers off the status page for the record | the instance |
| **`<END>`** | The stop, §3 | the instance |
| **`<END>` + 7 days** | The record's disposition is complete, §4. A closing message says what was kept and where | the map's channel |
| **`<END>` + 30 days** | The instance is deleted. The domain's A record is removed. The domain itself is kept — it is cheap and it is the only thing that makes a future map reachable at the same name | AWS, the registrar |

**Why the instance survives the ending by 30 days.** A stopped service that still
exists can be restarted; a deleted one cannot. Thirty days is time for the
retention rule to be executed carefully rather than in the same hour that people
are being told goodbye.

## 3. The stop itself, in order

```sh
# 1. The last reading. This is the run's own record of what it was at the end.
curl -s http://127.0.0.1:8796/api/status  > /var/lib/multiverse/final-status.json
curl -s http://127.0.0.1:8796/api/hops    > /var/lib/multiverse/final-hops.json
curl -s http://127.0.0.1:8796/api/species > /var/lib/multiverse/final-species.json
curl -s http://127.0.0.1:8796/api/history > /var/lib/multiverse/final-history.json

# 2. Stop the map first. The archive keeps recording until the last crossing is
#    through, which is the same ordering the restart policy uses to avoid a
#    ledger gap.
sudo systemctl stop multiverse-relay
sleep 30
sudo systemctl stop multiverse-archive

# 3. Stop them coming back. A wind-down that a reboot undoes is not a wind-down.
sudo systemctl disable multiverse-relay multiverse-archive
sudo systemctl disable --now multiverse-monitor.timer

# 4. The last backup, with the services quiet, so the ledger copy is a complete
#    file and not a prefix.
sudo -u multiverse /opt/multiverse/deploy/backup.sh --full

# 5. A final Lightsail snapshot, by hand, in the console. This is the copy that
#    outlives the instance.
```

Leave `multiverse-backup.timer` enabled until the record's disposition is done.
Leave nginx running. Publish a static page at `https://bibitesmultiverse.com/`
that explains the ending. This is kinder than a refused connection.

## 4. What becomes of the durable files

**Five files. This is the section Decision 3 is consumed in.** `MV_RETENTION` in
`deploy.env` selects an arm, and **the owner answered on 2026-08-12: arm B,
`prune-genomes`, with a 720-hour horizon.** Both arms are kept below because the
variable still selects and a later run may answer differently. Whichever it is,
**it is announced before anybody joins** — a participant is entitled to know what
becomes of the record of their world before they contribute to it.

### Both arms, unconditionally

| File | Disposition | Why |
|---|---|---|
| `peers.json` — the credential verifiers | **Destroyed** at `<END>` + 7. `shred -u` | Salted verifiers, never secrets, and worthless the moment the relay they authenticate against is gone. Keeping an authentication store for a service that does not exist is a liability with no upside. |
| `ring.json` — the slot register | **Kept with the record.** It is part of the archive's disposition, not the relay's | It is what makes the ledger legible: every record names slot numbers, and this is the only file that says which peerId held which slot at which coordinate. A ledger without it is a list of numbers. |
| The four `final-*.json` captures | **Kept and published** with the closing message | The run's last self-portrait: the map, its species, its flows and its totals. It is small, it is the thing participants will actually want to look at, and it costs one `curl` each. |

### Arm A — `MV_RETENTION=keep-everything`

The ledger, the genome store and `metrics.jsonl` are **kept whole**, moved off
the instance before it is deleted, and held by the owner as D11's seed of M7's
`species-catalog`.

- **The move:** the final Lightsail snapshot is the primary copy. Take a second
  by downloading `migrations.jsonl.gz`, `metrics.jsonl.gz` and a `tar` of
  `genomes/` to the development machine before `<END>` + 30. At the exit-test bar
  that is roughly 26 GB before compression; check the actual size and the
  destination's free space first — `dev_environment.md`'s disk budget exists
  because that check was once skipped.
- **What participants are told:** the record is kept; it is not published as a
  file dump; nothing in it was ever confidential — `join.md` states before anybody
  joins exactly what the map broadcasts about their world, and the archive holds
  that and nothing more. There is no removal-on-request mechanism and there never
  was: `contract-b-m4.md` §10 and §20 forbid eviction from the ledger and the
  genome store, the render deny list suppresses **the view and not the record**,
  and **that must not be promised to anybody**.
- **D6's graduation call:** recorded as *the archive becomes the catalog's
  ancestor in fact*. Note it in `system_decomposition.md`'s D6 row at the close.
- **The cost being accepted:** a disk bill that grew with the community's
  success — which, under a bounded commitment, was a bill with a stated end date
  rather than an open one.

### Arm B — `MV_RETENTION` bounds something

Three shapes, and the announcement names which:

| Rule | What is kept | What goes |
|---|---|---|
| `bounded-ledger` | The genome store whole; the ledger to the stated horizon | Ledger records older than the horizon, at `<END>` + 7 |
| **`prune-genomes`** — the chosen one | The ledger whole and forever; genomes to the stated horizon | Genome blobs not stored or served inside the horizon. **These go continuously, during the run** — see rule 2 — so by the ending the store already holds a horizon's worth and there is little left to do here |
| `graduate` | Whatever M7's catalog seeds from, extracted before the rest goes | The live archive as a live archive |

Rules for all three, and they are what keep a prune from being a betrayal:

1. **The horizon is announced before anybody joins**, not decided at the end.
2. **THE LEDGER IS NEVER PRUNED WHILE THE SERVICE IS RUNNING, AND THE GENOME
   STORE MAY BE.** This split is `contract-b-m4.md` §23, B33, and it is the one
   rule in this section that changed after the kit was built. `migrations.jsonl`
   keeps §10's no-eviction rule for the whole of the run without exception, at
   every setting of every knob: a ledger prune is a wind-down act performed on a
   copy, at `<END>` + 7, after the final snapshot. A genome **blob** under a set
   `MV_ARCHIVE_GENOME_HORIZON` is different — the archive evicts it during the
   run, on its own pass — because a horizon that only applied at the ending would
   buy no disk during the run, which is the whole reason to have one. **That is
   why the horizon is announced rather than merely recorded**: it takes something
   away while people are still playing, and rule 1 is what makes it honest.
   A hash whose blob is gone stays a lineage node in the record, permanently
   (§10, §23 B34).
3. **The prune is a filter into a new file, never an edit of the live one.** The
   original stays in the final snapshot.
4. **What was pruned is stated by count**, in the closing message. "We kept N
   records and Q genomes; the rest is in the final snapshot only" is an honest
   ending. Silence is not.

## 5. Extension, and the rule about shortening

**An extension is announced, and it is announced no later than `<END>` − 7.**
Update `MV_PERIOD_END` in `deploy.env`, re-issue the announcement text from
`ANNOUNCEMENT.md` with the new date, and post it everywhere the original went —
the release page included. An extension that some participants know about and
others do not is worse than no extension.

**The bound is never silently shortened.** If the run must end early — a cost the
owner cannot carry, a failure that cannot be fixed, a life event — then the
ending is still a **stated event**: an announcement with a date at least seven
days out, and the full §3 and §4 procedure. Risk 10's residue is one person with
a support load, and the honest answer to that is a shorter announced run, not a
quiet one.

**The one thing that is never done: letting it decay.** D24 exists precisely so
that the ending is *"a stated event rather than a service quietly decaying"*. A
relay that stops answering because nobody renewed a certificate is the failure
mode this whole document is written against.

## 6. The publish-the-relay path

D24 asked for this to be *prepared*, so that a map can outlive the owner's run.
It is prepared: **it is this kit.**

Everything a person needs to run their own map is in the repository —
`deploy/provision.sh` and this directory, the two binaries built by
`deploy/ship.sh` from `go/cmd/relay` and `go/cmd/archive`, and the two contracts.
The closing message should say so, in one line, with the link.

Three things to tell somebody who wants to take it on:

1. **The name is theirs, not yours.** Join strings carry the relay's advertised
   URL. A new operator mints new join strings on a new name, and every
   participant re-applies one. There is no transfer of a map; there is a new map
   that the same people join.
2. **A relay is cheap; an archive is not.** The relay is 93 MB resident and 0.045
   cores at eleven times a real map's rate — it fits on a permanently free
   instance. The archive is what needs a real machine, and `SIZING.md` is why.
3. **The obligations come with it.** All six of DQ2's: a supervisor, monitoring
   that reaches a person, backup of the irreplaceable files, a written restart
   policy, a name with a renewable certificate, and — if they want people to
   trust it — an announced period of their own.
