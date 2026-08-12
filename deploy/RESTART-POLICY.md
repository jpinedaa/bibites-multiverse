# Restart policy

**DQ2's fourth obligation, in its own words:** *"A restart policy, written down.
Which restarts are routine, what a participant should expect during one, and what
the operator does first afterwards."*

Everything below is written from measurement, not from custom. The measurements
are the living deployment's and the arithmetic is in `SIZING.md`.

**The one sentence to carry:** *restarting the relay is cheap and restarting the
archive is not, they are separate acts, and the archive's cost grows every day of
the run.*

---

## 1. The five kinds of restart

| # | What | Costs | Announce? |
|---|---|---|---|
| 1 | **Certificate rotation** | **Nothing. It is not a restart.** | No |
| 2 | **Relay only** — issuing join strings, a limit change, a relay upgrade | Seconds. Every peer reconnects and re-claims its own slot | For a planned one, yes |
| 3 | **Archive only** — a deny-list *file* edit is NOT this; a binary upgrade or a config change is | A full ledger replay, and **a hole in the record the width of the outage** | Yes |
| 4 | **Both, or a reboot** — kernel updates | The archive's cost, without the ledger hole | Yes, and schedule it |
| 5 | **Unplanned** — a crash, an OOM kill, ENOSPC | Whatever the above would have cost, plus the diagnosis | After the fact, always |

### 1. Certificate rotation is not a restart

B23's rotation row is a rule about what a rotation costs a connected peer, and
the answer is nothing. `GetCertificate` is called once per TLS handshake and
stats the two files, so a renewed pair is picked up by the **next handshake** —
no signal, no restart, no reload command, and an established session is
untouched. This was implemented and tested in WP2, and `tls-deploy-hook.sh` is
built around it: the hook installs the pair and reloads **nginx** only, because
nginx is the one process here that holds a certificate open.

A half-written renewal — the certificate new and the key not yet, for the instant
between two writes — makes the load fail, and the reloader **keeps serving the
certificate it already has**. An expired-in-a-month certificate beats no
certificate now. The hook additionally writes through temporaries and renames, so
the case does not arise.

`monitor.sh`'s **cert** check compares the certificate the listener *serves*
against the copy on disk, which is how a deploy hook that silently stopped
running is caught before the certificate expires.

### 2. The relay-only restart

**Cost: seconds.** The relay is 93 MB resident, flat, with no replay and no state
to rebuild beyond reading `ring.json` and `peers.json`.

**What a participant sees:** an ordinary disconnect. Their sidecar reconnects on
its backoff ladder and re-claims its own slot with `reason: "reclaimed"`. Slot
numbers, coordinates, reservations and credentials all survive — they are on
disk, not in the process.

**The one real cost, and it is worth stating plainly to participants:** a relay
restart drops **every outstanding forwarded record**. Those entries fall back to
the sender's bounded 24-hour hold instead of re-routing in seconds. Nothing is
lost; some crossings are slow for a while. The living deployment's own crossing
window measured the shape of this: taking the map down takes the sidecars with
it, so their journals replay-flush and there is no custody burst — peak 4 against
55 for a rolling deploy.

> **This is the item WP3 is changing.** The forward receipt (B26) turns the
> sender's own journal into the evidence, so a restart stops costing a 24-hour
> hold. It ships alongside this kit. Until it is deployed, the sentence above is
> the truth and belongs in the participant-facing statement.

**When it is required:** issuing a join string. Minting is a startup command and
a serving relay is a second writer of `peers.json`. `issue-join.sh` takes the
restart deliberately, and **batches**: one restart for five participants.

### 3. The archive-only restart — the expensive one

**Cost: a full ledger replay, and it grows with the ledger.**

    replay seconds  ≈  ledger records ÷ ~40,000 per second
    peak memory     ≈  ledger records × 1.30 KB

The 40,000/s is the development host's, it **will be slower on a cloud vCPU**,
and it is corroborated by the living deployment's own two measurements: **~93 s
at 3.7 M records** on 2026-08-10 and **~150 s at 6.24 M records** on 2026-08-11 —
41,600/s, and growing in absolute terms by the day. Every recorded replay figure
expires the day after it is written: measure from `wc -l` on the day.

At the exit-test bar that is roughly:

| Day of the run | Records | Replay | Peak RAM |
|---|---|---|---|
| 7 | 1.7 M | ~45 s | 2.2 GB |
| 30 | 7.3 M | ~3 min | 9.4 GB |
| 90 | 21.8 M | **~9 min** | **28 GB** |

**AND AN ARCHIVE RESTART ON A LIVE MAP COSTS A LEDGER GAP EQUAL TO ITS DOWNTIME.**
The archive is a subscriber. While it is replaying it is not subscribed, and
every crossing that happens in that window is **never recorded by anybody** — no
peer and no relay holds a copy of the archive's record. Nine minutes of replay on
day 90 is nine minutes of the record that does not exist.

**This is measured, not feared.** The living deployment's last archive restart
cost **1,940 crossings, absent from the record forever**. That is §5.1 working
exactly as designed — the traffic itself is untouched and only the record has the
gap — and it is the reason the ordering below exists.

The avoidance is known and was executed once already: **restart the archive
inside a map outage.** The 2026-08-11 crossing brought the relay down, restarted
the archive inside the window, and the archive re-subscribed 880 ms before the
first sidecar came back — **zero ledger gap**. If the archive must restart and
the record matters, stop the relay first and start it last.

**Therefore: batch the reasons. Never restart the archive to collect one number.**
`m5_tracking.md` already holds a debt against the archive's next restart (WP4's
deny-list flag and the `limits` key on `/api/status`, plus the status page's
gzip). Add to the debt; do not pay it twice.

**What does NOT need a restart:** the render deny list. `/etc/multiverse/deny-list`
is re-read in place, so moderating costs an edit.

**What the operator does while it replays:** nothing to the relay. The map is
running, crossings are flowing, participants are unaffected except that the
status page is unavailable. Watch the archive's log for the subscription line;
`monitor.sh` will report `archive-healthz WARN` with "may still be replaying"
rather than a false outage.

### 4. The reboot

Security updates apply themselves; **reboots do not**, deliberately. An automatic
03:00 reboot would replay the archive unannounced and cost a ledger gap. A
pending kernel shows up in `monitor.sh`'s **reboot** check.

The procedure:

1. Announce it. A kernel reboot is a case 3 restart with extra steps.
2. Take a Lightsail snapshot by hand.
3. `sudo systemctl stop multiverse-relay` — the map is now down, which is what
   protects the ledger.
4. `sudo reboot`
5. Both units come back at boot (`WantedBy=multi-user.target`). The archive
   replays with no map running, so nothing is missed.
6. Run the post-restart checks in §3 below.

### 5. The unplanned restart

`Restart=always` with `RestartSec=5s` (relay) and `15s` (archive). The limits are
deliberate: 10 restarts in 5 minutes for the relay, 6 in 10 minutes for the
archive. **A process that exceeds them enters `failed` and systemd stops trying.**

That is not an oversight. An archive that cannot complete its replay would
otherwise loop forever, saturating the box's single spare core and doing nothing.
Something has to notice, and the thing that notices is `monitor.sh`'s **units**
check, which alerts a person. **There is no auto-heal beyond systemd's own
limits**, because a service that hides its failures from its operator is worse
than one that stops.

Under memory pressure the kernel takes the archive first: `OOMScoreAdjust=-500`
on the relay, `+200` on the archive. Losing the archive costs a ledger gap;
losing the relay costs the map.

---

## 2. Before any planned restart

1. Read `monitor.sh --verbose`. If **replay** is already CRIT, **do not restart
   the archive** — it may not come back. Fix that first (`SIZING.md` §7).
2. Measure the replay: `wc -l /var/lib/multiverse/archive/migrations.jsonl` ÷
   40,000 is the optimistic floor.
3. `sudo systemctl start multiverse-backup.service` — a fresh identity snapshot.
4. For anything touching the archive: a Lightsail snapshot by hand.
5. Announce, if it is case 2, 3 or 4.

## 3. The first five things afterwards

In this order, because each answers a question the next one assumes:

1. `systemctl is-active multiverse-relay multiverse-archive` — both `active`.
2. `curl -s https://<domain>/healthz` — the map answers.
3. `curl -s http://127.0.0.1:8796/api/status | jq '{relayConnected, haveStatus, statusAgeMs}'`
   — **`relayConnected: true` is the one that matters.** An archive that is
   running and not subscribed records nothing and complains to nobody.
4. `... | jq '.totals'` — every peer back, `liveSlots` at its pre-restart value.
   Peers return on a backoff ladder, so give it a couple of minutes before
   calling one missing.
5. `jq '.peers | length' /var/lib/multiverse/relay/peers.json` — the verifier
   store is intact. This is the check nobody thinks to run and the one whose
   failure costs a handover per participant.

Then: `monitor.sh --verbose`, and confirm the alert channel is not sitting on a
stale CRIT.

## 4. What participants are told

The text below is the participant-facing half this policy owes. It is reproduced
in `ANNOUNCEMENT.md` for the documentation slots to consume.

> **Restarts are routine and they are short.**
>
> The map's relay restarts from time to time: to hand out a join string to a new
> participant, to apply a setting, or to take a security update. **You do not
> need to do anything.** Your sidecar notices the disconnect, waits, reconnects,
> and re-claims your own slot at your own coordinate. Your slot number, your
> position and your credential are unchanged — they live on disk, not in the
> running process.
>
> What you may notice: for a short while after a restart, organisms that were
> mid-crossing take longer to arrive. They are not lost. They are held by *your*
> machine and re-sent, and the hold is bounded at 24 hours.
>
> A restart of the **archive** — the thing that draws the map's status page —
> takes longer, and while it happens the page is unavailable. The map itself
> keeps running the whole time; crossings continue.
>
> Planned restarts are announced. Unplanned ones get an explanation afterwards.

## 5. The replay is on a clock, and the clock is the run

The last thing this policy has to say is the uncomfortable one. **Replay time and
replay memory both grow linearly with the ledger, for the whole announced
period.** A restart that costs 45 seconds in week one costs nine minutes in month
three, and somewhere past that the archive stops being able to restart at all on
the memory it has.

That is not a bug to fix inside the run. It is the shape of an append-only record
that nothing may evict from, and it is precisely why Decision 3 exists and why
its deadline is D24's announced ending. This policy's job is to make the cost
visible in advance — `monitor.sh`'s **replay** check — rather than discovered
during an outage.
