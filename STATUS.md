# Project status

Last updated: 2026-08-25 UTC.

Bibites Multiverse `0.3.7` is public. The first announced service period runs from
**August 14 through November 14, 2026**, and reminders begin 30 days before the end.

## Current public phase

M5 delivered the public release, hosted service, participant packages, and support guides.
The public experiment now collects the evidence for the M5 exit test.

The exit test requires all of these results:

- At least four participants who are not the project owner.
- At least 72 hours of continuous operation.
- No operator action on a participant's computer.

Use the [production-health dashboard](https://bibitesmultiverse.com/health) for current test,
service, host, and coverage results. Use the
[live map](https://bibitesmultiverse.com/live) for current worlds and migrations.

## Current production health

The production-health dashboard is live. It organizes production into one system map and four
visual layers: Compute, Cloud and services, Application and traffic, and Archive and data. Eight
headline signals show checks, worlds, migration flow, records, gaps, CPU, memory, and disk. Host
and TCP charts use the bounded two-hour service-host window. Check matrices, world grids, service
memory bars, pipeline stages, and a coverage strip show the other live results. Tooltips explain
each static and dynamic visual. Full charts show UTC x-axis values and exact values under pointer
or keyboard inspection.

Production runs exact public feature commit [`c9e4f3a`](https://github.com/jpinedaa/bibites-multiverse/commit/c9e4f3a2601fda3dd631eec018758e99c0952162).
That exact build is live. Commit [`2564ea1`](https://github.com/jpinedaa/bibites-multiverse/commit/2564ea19f86898ffe756b4c9dfe08d910c45190e)
makes a stopped traffic producer publish gaps instead of reusing an old request window.
The correction is not yet deployed.
The `2026-08-24T19:40Z` closeout verified Linux pressure, VM events, public request rate, and HTTP
status classes. It also verified nginx p50 and p95 duration, fixed route groups, and archive input
rate. Host verification passed 34 checks with no failure. All eight public routes returned HTTP
`200`. Two complete post-change samples passed the API shape and privacy checks.

The newest complete sample reported CPU busy at `13.1%` and available memory at `40.84%`.
Public traffic was `2.53 requests/s`, with p50 at `1 ms`, p95 at `5 ms`, and no server errors.
The archive received `33.88 records/s`. The guarded activation gated participants for 71 seconds.
Twelve of 19 slots were live after reconnection.

The overall result remains critical. Data-volume use is `90%`. The old archive-memory check also
models every permanent record as resident memory. The current archive uses bounded roll-up and
duplicate-window state, so that model does not describe its current memory shape. The dashboard
shows both critical results.

The dashboard is an on-host current view. It is not an external dead-man check or an off-host
history store. Gray gaps identify public probes, centralized world-host performance, and
continuous profiles that no current feed supplies.

### Zero-lane refusal progress

On 2026-08-22, protected five-minute lane samples showed a localized migration-flow problem.
Specific live, populated sources had every open outbound lane at `0/min` in repeated samples,
although aggregate migration across the map continued. One lane can normally show zero when no
organism crosses during one window. Repeated zero values across every open outbound lane from a
live, populated source are not a normal lull.

An earlier correction paced relay writes and participant acknowledgements. It removed the burst
and capacity-disconnect path. Five Windows worlds and six hosted Linux worlds retained their
identities and journals when that correction was deployed. Two exact cloud closeout intervals had
positive aggregate migration and no hosted source with every open outbound lane at zero.

On 2026-08-24, a sanitized live diagnosis bounded the remaining fault on slot 7. All four open
outbound lanes stayed at zero while the slot was live, populated, and connected to its mod. The
map still recorded `729/min` in aggregate. The source's outbound custody depth stayed at its limit
of 64, its paced depth was zero, and its cumulative lost-forward counter was 5,461. The source
could not accept another export. This was a custody-capacity wedge, not an empty world or a slow
delivery queue.

The cause was an ambiguity in the relay's old `neverForwarded` proof. That proof covered the whole
migration. It could not identify a queue refusal for the current attempt after an earlier attempt
had reached a different peer. The sidecar correctly refused to reroute on that ambiguous proof.
Enough such entries could fill all 64 custody positions.

Release `0.3.7` adds an optional Contract B 4.2 `refusedAttempt` proof. The proof binds the relay
session, destination slot, and reroute count. The sidecar acts only when all three match its durable
current attempt and no forwarding receipt contradicts the refusal. It records the tried slots and
one refusal deadline, continues on the same axis to a distinct compatible destination, and bounces
once when the bounded walk is exhausted. A relay-drain refusal carries no attempt proof and cannot
move a migration.

The durable sent transition now occurs before socket enqueue. A crash can therefore lose one
migration, which the at-most-once contract permits, but it cannot duplicate one. Restart,
reconnect, replay, compaction, and a repeated or stale refusal cannot reset the bound. An upgrade
regression starts with 64 pre-4.2 refused entries and proves that one exact 4.2 refusal advances
each entry once to a different destination.

Contract B 4.2 is an optional-minor extension of the existing `/contract-b/v4` endpoint. The
hosted minimum remains unset, so 4.0 and 4.1 peers still join. The active hosted relay and archive
already speak 4.2 at exact source `c9e4f3a`. The controlled slot 7 and cloud-world sidecar updates
remain planned. This relay-first order means an old participant can omit the new proof, but no new
participant must guess whether a refusal belongs to its current attempt.

### Population-aware portal routing rollout

Release `0.3.6` changes the pre-custody overload path. A live destination that explicitly refuses
a migration no longer receives the next offer merely because it remains live. The source records
that refusal durably and continues in the same direction to the next compatible world it has not
tried. Queue overload and the new population policy both use this custody-safe path; relay queue
pressure remains a retry to the same destination and is not misclassified as a world refusal.

The population controller ships to participants in `adaptive`. It measures a per-world capacity
estimate for ×10, publishes the limit and open/closed decision, and refuses a new arrival on that
decision once the estimator is ready; before readiness it fails open. `adaptive-shadow` keeps the
identical measurement and publication while refusing nothing, and is the one-variable rollback.
The default moved from shadow to enforcing on 2026-08-25; the reasoning, what it does not change,
and the review it is under are in
[`docs/population-admission.md`](docs/population-admission.md).

On 2026-08-23, candidate `94a71b2` passed the full uncached Go suite and vet, a release plugin
build, Windows and Linux cross-build checks, and the three-live-world spillover regression. Five
Windows worlds and six hosted Linux worlds then activated mod `0.6.8` and the candidate sidecar.
All eleven retained their reserved slots with mod and relay connected, exposed requested speed and
non-enforcing shadow state, and reported zero capacity sheds and zero discarded journal bytes.
The guarded cloud runtime transaction and host verification completed successfully. The race suite
was not rerun because this WSL image has no CGO compiler; forced rollback, disk failure, crash
replay, and private-map behavior were not exercised in production.

Later on 2026-08-23, the operator authorized an early controlled-world exception to the 24-hour
shadow gate in order to collect real enforcement evidence. The five Windows worlds now persist
`MULTIVERSE_INBOUND_ADMISSION=adaptive`, and the six hosted Linux worlds receive the same override
from their systemd unit. The hosted runtime transaction activated immutable runtime
`ef3ff4d5a3692592b2ca582b921ee70596d6f1ace594a98312a64c6948a47e2b`; SSM command
`d98dc8aa-f9b4-4581-9321-0a181c7c88ae` succeeded on all six worlds, host verification passed, and
the three temporary credential parameters were deleted. The initial public closeout had all
eleven controlled worlds live, mod-connected, and enforcing, six closed by their learned limits,
continuing migrations, and no capacity sheds or discarded journal bytes. Existing learned state
was retained. Automatic `metrics.jsonl` history will be reviewed after 24 and 72 hours; rollback
criteria and fixed host-class fallbacks are recorded in the population-admission runbook.

The first 20-minute enforcement watch also established the loss baseline that later reviews must
use. Slot 3 recorded 22 unanswered forwards: 15 were sent to newly closed hosted slot 6 during a
short post-activation burst, and the later seven targeted participant slot 19 around a mod
disconnect. Other hosted sources added only isolated losses. This was not classified as a new
enforcement regression because the retained pre-promotion logs contain similar clustered
five-minute timeouts from slots 2 and 5 to overloaded local slot 9 while it was still in shadow
mode. All affected sends had relay forward receipts, so the follow-up is to determine why no ACK
or pre-custody NACK returned from those destinations, and whether offers to a recently
mod-disconnected slot can be narrowed without guessing about custody. Enforcement remains active;
the 24-hour comparison must use rates on each side of the promotion rather than restart-reset
aggregate totals.

The same watch exposed an operator-surface defect rather than an admission defect: the live map
animated copied `MIGRATION_PAYLOAD` offers immediately, before the destination answered. Slot 9
could therefore be visibly "entered" even while its enforcing gate NACKed every new offer, and the
map's migration-ID dedup could hide the later accepted reroute. The archive now correlates each
offer with the destination peer's `MIGRATION_ACK`, which is emitted only after the game accepts the
spawn. Rejections do not animate, reroutes resolve to the peer that actually ACKs, and the page no
longer guesses arrivals from payload-derived counters if the hop feed is unavailable. The lane
rate remains explicitly labelled as migration offers, so routing pressure stays visible beside a
closed gate without being presented as delivered population.

An earlier status entry attributed a separate service-host activation to `c321aad`. It reported a
75-second archive replay, a 78-second participant outage, and archive subscription before every
placement claim. Its 60-second closeout had the same 15 live and four dark slots and a connected
relay. All four public routes returned HTTP `200`. The public status reported 11 mod `0.6.8`
worlds in `adaptive-shadow` and zero enforcing population gates. The live page carried the new
population-admission explanation, and the announcements page published the planned window and
actual return time.

A later direct binary audit identified `60dac3a` instead. Commit `c321aad` is not an ancestor of
that source, and no retained receipt proves that `c321aad` ran. The earlier source attribution
therefore remains unreconciled.

The confirmed-hop correction merged and deployed on 2026-08-24 as `3617390`; GitHub checks run
`32681067865` completed successfully. A fresh provider snapshot and local durable-file backup
preceded the guarded restart. The archive replayed in 97 seconds, and the peer gate held the
participant outage to 104 seconds. The relay log proved the archive subscribed before any
placement claim. At the 60-second closeout, the original 15 live and four dark slots were present
and the relay was connected. The public page carried the confirmed-delivery wording, and the
bounded hop endpoint was active.

At `2026-08-24T02:25Z`, a fresh read-only host receipt reported active relay and archive units.
Their installed hashes exactly matched a clean `3617390` build, and the deployment gate passed all
34 checks. Commit `3617390` is therefore the current service and rollback baseline before the
Contract B 4.2 rollout.

The production regression interval separated the two signals directly. During 15 seconds, slot
9's enforcing closed gate increased `admissionRejectedTotal` by 27. The `/api/hops` endpoint
contained 148 new ACK-confirmed deliveries elsewhere and none to slot 9. Its `pacedDepth` stayed
at 12. Those arrivals entered custody before the population gate closed and remain there until
delivery. A later animation into a closed world can therefore be correct only for that accepted
backlog. The rejected new offers no longer animate.

A follow-up visual audit found that the confirmed-hop backend was correct but the browser still
attached every glyph to the source edge's currently drawn lane. When that ordinary lane ended at
closed slot 9 and the ACK named a later world, the glyph visibly entered slot 9 and only the later
world flashed. The live-console correction records the exact ordered receiver refusals in the
bounded hop correlator. Each map cell now shows its fresh effective population limit and explicit
gate state. A confirmed spillover stops at the refused boundary with a refusal label, then follows
a temporary dashed route to the ACK-confirmed world. Standalone rejections still do not animate.

That live-console correction merged as `4ade74e` and passed GitHub checks run `32729843166`.
A fresh provider snapshot and the service's durable-file backup preceded activation. The guarded
archive restart replayed in 86 seconds and held the participant gate for 88 seconds; the archive
subscribed before the gate reopened. The 60-second closeout reported 14 live and five dark slots
with the relay connected. Installed and running archive/relay hashes matched the staged build, and
the host verifier passed all 34 checks.

The first post-restart `/api/hops` sample contained 60 confirmed deliveries with an explicit
receiver-refusal chain. One example went from slot 5 to slot 4 only after slots 6 and 14 refused
it. This is the evidence the page now renders as stop, refusal, and dashed reroute. At
`2026-08-24T13:06Z`, slot 9 instead reported `adaptive`, sample count zero, effective limit
unknown, estimate 10, and enforcement false. Its visible arrivals are therefore correctly marked
`OPEN · LEARNING`; they are not evidence that a closed enforcing gate was bypassed.

### Species genealogy repair

The 2026-08-22 raw-record rebuild removed the species aggregate overflow. It folded
`25,400,734` records in `406.39 s` and restored the later species observations. The rebuild
reduced the living rows with no parent evidence from 14 to one excluded seed species.

A second defect remained in the genealogy view. The archive used the normalized species name as
the identity of a family-tree node. The game can reuse a name for a later species. A later parent
claim then replaced the earlier edge for that name. Read-only analysis found 99 names with more
than one parent claim. It also found name cycles of 4 and 50 nodes. In a current sample, 24 of 26
living ancestry walks reached one of those cycles. The tree guard cut those walks, so related
living species appeared as separate roots.

The source repair keeps an immutable lineage instance for each recorded name and parent path. A
reused name creates another instance instead of replacing an earlier edge. The API and page show
reused names, unresolved identities, and capacity or cycle guard hits separately. Roll-up format
3 persists this graph. An archive with format 2 must rebuild the fold from the ordered raw record.

Public commit `60dac3a` went live on 2026-08-23. The archive rebuilt all `29,846,555` raw records
into roll-up format 3 in `598.792 s`. The guarded participant outage was 10 minutes 24 seconds,
which was 24 seconds longer than the maintenance notice estimated. The archive and relay returned
on the new source with no record gap.

The first live check reported 19,700 lineage instances, 5,269 reused names, and two unresolved
identity links. Ledger and lineage overflow were zero. Cycle, walk, and node-cap guards were also
zero. Remaining roots now distinguish seed stock, missing raw ancestry, recorded family roots,
and genuinely ambiguous evidence. They are not derived cycle cuts.

## Hosted service

This section states the terms the hosted map operates under. It is not a health report.
Every service notice — planned work, a change of terms — is published on the
[announcements page](https://bibitesmultiverse.com/announcements/).

| Item | Current state |
|---|---|
| Network protocol in force | The active hosted relay and archive speak `contract-b/4.2` at exact source `c9e4f3a`. Older worlds that speak 4.0 or 4.1 still join because the new attempt proof is optional. The controlled slot 7 and cloud-world sidecar updates remain planned. |
| Crossing between worlds | **At-most-once.** A world hands an organism over once; if it does not arrive it is lost, the loss is counted, and nothing re-sends it or brings it home. Both halves are now deployed: the service since 2026-08-17, and the participant half with the current release |
| Record retention | Three periods, in force since 2026-08-17. See below |

**The map keeps three things for three different times.** The
[participant guide](docs/participant/join.md) states this in full and is the page to read first.

- **For the whole run and beyond**, the record itself: every species that crossed, how often, when
  it first and last crossed, and which species it descended from. Nothing removes a species, a
  count, or a family link.
- **For 30 days**, the individual crossing lines behind those counts. After 30 days a line is no
  longer on the server; the crossing is still counted and the family link is still there.
- **For 30 days**, the related genome files.

A compressed copy of the crossing lines is kept away from the server for the length of the run, and
a line only leaves the server once that copy is confirmed. **This is a capacity schedule and not a
deletion service**: it ages lines by date and cannot pick out a world, a species, or an organism.
[`docs/participant/leave.md`](docs/participant/leave.md) states what that means for a removal
request.

## Public release

| Item | Public state |
|---|---|
| Release | [`v0.3.7`](https://github.com/jpinedaa/bibites-multiverse/releases/tag/v0.3.7) |
| Supported game | *The Bibites* `0.6.3.1` |
| Plugin | `0.6.8` |
| Mod-to-sidecar protocol | `contract-a/2.4` |
| Network protocol | `contract-b/4.2` — what this release and the active hosted service speak. See [Hosted service](#hosted-service). Worlds on `contract-b/4.0` and `contract-b/4.1` still join. |
| Windows package | Single setup executable with the authorized portable game, application shortcuts, and uninstall registration. An existing game is optional |
| Windows launcher | The shortcuts open `BibitesMultiverseLauncher.exe`, a window that lists every world on this computer with what is running, whether its mod has reached the map, its speed and its slot, and starts, stops, creates, clones, deletes and diagnoses them. A per-session switch runs one session with or without a window. The same launcher's commands and console menu ship beside it as `multiverse-launcher.exe`, which is what a script calls. One game folder supports five worlds at once |
| Linux package | Complete archive with the authorized native game; an existing-game add-on remains available |
| Public-map setup | Every participant package includes `public-map.json`. Installation creates a unique credential over HTTPS |
| Homepage | Links the Windows setup, the Linux complete package, and the checksums of the newest release, and names which release that is. Neither the links nor the number carry a compiled-in version — the links address `/releases/latest` and the number is resolved from GitHub hourly in the background — so a release reaches the homepage with no deployment, and the page drops the number rather than guess if that lookup fails. A three-step Windows walkthrough sits under those links, with one line for Linux |
| Live console | Full-screen map fitting, per-world live population-gate badges and effective limits, evidence-backed refusal/reroute playback, visible-range brain and population charts, all-time and 24-hour population views, shared navigation, and a live homepage status light are implemented. The Species tab expands vertically with the page. Wide timeline content keeps its horizontal scroll inside the drawing. The genealogy view separates reused names into immutable lineage instances and labels unresolved evidence. |
| Broadcast world | The broadcast page names the world on camera and draws its place in the map grid. The live map badges that world on its map cell, in the worlds table, and in its settings card, and every badge links back to the broadcast. |
| Release process | Every pull request and every push to `main` runs the project's automated checks, and a version tag builds and publishes the release from the owner's own machine. The homepage follows the published release on its own |

The release page provides checksums. The participant installer compares the game build with
the support matrix before it changes the game directory, and it refuses an install over a world
that is still running before it has changed anything. By default, the Windows GUI starts the
connected world and opens the installed launcher after installation. A world starts at ten times
the game's own speed, which the in-game speed slider still moves. The launcher runs a world
headless, and a stop asks the world
to save and quit through its own mod, so a headless world loses nothing either; the generated
start script keeps its own headless switch and the same stop.
The packaged public join configuration contains service addresses, not a shared world secret.
Release `0.2.1` added a Linux complete package and automatic enrollment to the Linux installer.
The releases after it added the single-file Windows setup and normal application shortcuts.
Release `0.2.4` added the Windows launcher application and more than one local world per computer,
on top of that setup and those shortcuts.
Release `0.2.5` is a Windows install fix: the installer and the uninstaller no longer refuse when
another copy of the game is running under an account this one cannot inspect. Only a copy running
from the folder being written to stops them.
Release `0.2.6` makes installing again over an existing data root keep that world: the installer
adopts the identity already there instead of refusing with `INS-ENROLL`, and an uninstall keeps
the credential with the journal.
Release `0.2.7` ships the `0.6.5` plugin (durable fertility setting), reads a world's identity from
its own sidecar log when the record is gone, and is the first release built and published by the
release workflow.
Release `0.2.8` lets the installer finish over a data root whose secret no world ever used, keeping
that secret aside as an orphan and enrolling a fresh identity.
Release `0.3.0` makes the Windows launcher a window: every world on the computer, what each one is
doing, whether its mod has really reached the map, and one button to start or stop it. The same
launcher's commands ship beside it as `multiverse-launcher.exe`, which is what a script calls. In
this release a world also starts at ten times the game's own speed, every stop asks the world to
save and quit through its mod so a headless stop loses nothing, the setup refuses an install over a
running world before it changes anything, the sidecar's journal is bounded on Windows for the first
time, and the world speaks `contract-b/4.1`, where a crossing that does not arrive is counted
rather than held.
Release `0.3.1` fixes the install-again-then-uninstall cycle that could leave `0.3.0` unable to
install again: the setup owns the mod framework inside this package's own copy of the game, repairs
a game folder an earlier uninstall emptied instead of refusing it, and the uninstall takes that copy
back whole — framework, log and cache with it — along with the launcher profile and the application
folder holding it, so *Installed apps* removes the entry it registered. The Windows uninstall also
keeps its ledger as a file, `<data root>\logs\uninstall-<utc>.log`, because an uninstall started
from *Installed apps* leaves no window to read afterwards.
Release `0.3.2` makes a setup run over an installation that is already there an update rather than
a reset: the world keeps the name, port, headless setting and selection the participant gave it,
an upgrade over a Steam or itch.io game keeps the mod framework removable from that folder, and a
program file an earlier release left behind is swept by hash. The launcher also says when a newer
release has been published — one anonymous background request to the project's own homepage, which
nothing waits on, which is silent when it fails, and which `MULTIVERSE_NO_UPDATE_CHECK` turns off.
Release `0.3.3` opens the installed graphical launcher after the default Windows setup starts the
connected game. If Windows cannot open it, the installation stays complete and the world stays
running. Setup shows the launcher's path for another try.
Release `0.3.4` fixes migration queues that could appear to block blue portals after a restart or
traffic burst. The relay paces aggregate migration fan-in per destination, and the participant
sidecar drains durable acknowledgements and payloads under the published frame budget without
repeated capacity disconnects. Existing identities, saves, journals, and queued organisms are
preserved during the update.
Release `0.3.5` fixes Linux complete-edition upgrades and removals. The Linux `0.3.4` installer or
uninstaller sometimes stopped when it read a large install record. Release `0.3.5` changes no game,
plugin, sidecar, or protocol version.
Release `0.3.6` makes a live world's pre-custody overload refusal send the organism onward to the
next compatible world it has not tried, rather than repeatedly offering it back to the same full
destination. It also adds fixed and adaptive living-population admission, with adaptive estimation
deployed in non-enforcing shadow mode while production evidence accumulates, and publishes the
decision in local and live status views. Mod `0.6.8` reports requested speed separately from
achieved speed so the estimator does not mistake a deliberately slower world for an overloaded
machine.
Release `0.3.7` prevents exact relay queue refusals from filling all 64 outbound custody positions.
Contract B 4.2 identifies the refused attempt, and the participant records a bounded same-axis walk
before it tries another compatible destination. The journal remains the custody record throughout
an upgrade; no identity, save, or queued organism is cleared.
Private maps still accept a private join-string file on both platforms.

**The defect `0.3.1` fixes, and what a computer `0.3.0` already stranded needs.**
Release `0.3.0` as downloaded can leave a computer unable to install it again. Install it, install
it again over the same complete-edition game copy, then uninstall: the second install reads the mod
framework inside that copy as somebody else's, the uninstall leaves the framework there and removes
the game from around it, and the setup after that finds a game folder with no game in it and
refuses to overwrite it — `INS-RUNTIME`. Its complete-edition uninstall also leaves a launcher
profile and the application folder holding it behind whatever the shape of the install, so
*Installed apps* cannot take that entry away.
**Install `0.3.1` or later over it and there is nothing to delete by hand**: the setup names what it found in
such a folder, removes it whole and unpacks the game again, and its own uninstall reclaims that
copy — framework, log and cache with it — with the profile and the application folder, so the cycle
strands nobody.
**Deleting that game copy by hand is the same act, and is safe for the same reason**: the one folder
`%LOCALAPPDATA%\BibitesMultiverse\runtimes\<sha>` holds this package's own copy of the game and
nothing of yours — your worlds are the game's own saves, and this world's journal, logs and
credential are in the data root beside that folder rather than inside it. Both rows are in
[`docs/error-taxonomy.md`](docs/error-taxonomy.md), with the uninstall ledger their remedies name.

## Milestone state

| Milestone | State |
|---|---|
| M1 — in-game round trip | Complete |
| M2 — two simulations on one machine | Complete |
| M3 — remote LAN ring and archive | Complete |
| M4 — resilient grid and observability | Complete |
| M5 — public release | Release delivered. Public evidence phase open. |
| M6 — direct peer-to-peer transport | Future work |
| M7 — ecosystem completeness | Future work |

Read the [system design](system_decomposition.md) for milestone scope. Read the
[frozen M5 record](m5_tracking.md) for delivery evidence and the open exit-test bar.

## Follow the project

- Read the [participant guide](docs/README.md) before you connect a world.
- Open the [public map](https://bibitesmultiverse.com/live) to explore migrations and lineages.
- Open the [production-health dashboard](https://bibitesmultiverse.com/health) to see current
  production tests, service health, host performance, and coverage gaps.
- Read the [announcements page](https://bibitesmultiverse.com/announcements/) for service notices.
  It is the only channel the service publishes them on.
- The optional [shared broadcast](https://bibitesmultiverse.com/watch) follows one participant
  world on the map. It is relevant only when a publisher is available.
- Use [GitHub issues](https://github.com/jpinedaa/bibites-multiverse/issues) for defects and
  experiment proposals.
