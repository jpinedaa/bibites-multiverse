# Project status

Last updated: 2026-08-23 UTC.

Bibites Multiverse `0.3.6` is public. The first announced service period runs from
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

The production-health dashboard is live. It joins the scheduled five-minute checks with a bounded
two-hour view of the service host. It also shows current worlds, record integrity, restart cost,
deployment identity, and broadcast audience state.

At the `2026-08-22T00:06Z` closeout check, availability and record-integrity checks passed.
All six public routes returned HTTP `200`. Sixteen slots were live, and one previously dark slot
stayed dark. The dashboard reports the overall result as critical because the old archive-memory
check still models every permanent record as resident memory. The current archive uses bounded
roll-up and duplicate-window state, so that model does not describe its current memory shape.
The dashboard shows this known critical result without weakening or hiding it.

The dashboard is an on-host current view. It is not an external dead-man check or an off-host
history store. Its Coverage section names the missing tests and profilers.

### Localized migration-flow correction rollout

On 2026-08-22, protected five-minute lane samples showed a localized migration-flow problem.
Specific live, populated sources had every open outbound lane at `0/min` in repeated samples.
Aggregate migration across the map continued throughout the sampled period.
One lane can normally show zero when no organism crosses during one window.
Repeated zero values across every open outbound lane from a live, populated source need
investigation.

The same period contained overload responses and relay capacity shedding from several destination
paths.
Code review found that migration traffic from many sources can aggregate at one destination.
A destination response burst can then reach the per-connection frame limit.
The relay can then close that destination connection.
The protected observations correlate with this mechanism, but they do not prove that it caused
every zero lane.

Public commit `5ca9bef` paces physical migration writes for each destination identity.
The relay refuses a migration before it creates a forwarding record when the bounded destination
queue is full. It keeps that destination connected.

On 2026-08-23, operators installed and activated this relay correction. The relay service stayed
healthy, but the mandatory relay-only canary did not meet every gate. Later review assigned the
failed signals to participant-owned sidecar state, not relay transport. Isolated tests reproduced
the visible pattern with a saturated journal. They also confirmed that the current sidecar recovers
it. The service did not inspect any external participant's executable, timeout, clock, or journal
state. Thus, the exact participant-local cause remains open. Release `0.3.4` completes the
correction on the participant side. It paces journal-backed payloads and acknowledgements, drains
older acknowledgements first, and keeps ordinary control traffic responsive. The existing journals
remain the custody record and must not be cleared.

Release `0.3.4` was published and then deployed on 2026-08-23. Five Windows worlds on the release
machine retained their identities and journals through the upgrade; all five rejoined their
reserved slots with the new sidecar binary. The guarded cloud runtime transaction then activated
the same source on all six hosted worlds and verified the installed binary against the candidate
hash. All eleven worlds were live, game-connected and relay-connected after their respective
health windows. Every one reported zero relay capacity sheds and zero discarded journal bytes.
One Windows world's 50-entry durable acknowledgement backlog drained to zero after restart, while
the pacing deferral counters advanced. The public health and status routes returned HTTP `200`,
and the status route reported a connected relay.

The release-machine sample also records why a paced delivery queue can remain nonzero without a
new relay fault. Five concurrent games requested 10× speed, but some were achieving only about
1.2× to 5.1×. Delivery pacing is deliberately measured in the receiving world's simulated time,
so those CPU-limited worlds drain more slowly in wall-clock time. The new relay pacing prevents
aggregate fan-in from becoming a burst or a capacity disconnect; it does not make an overloaded
simulation advance faster and it does not erase its durable queue.

Two exact five-minute cloud closeout intervals kept aggregate migration positive. All six hosted
sources stayed live and mod-connected. None had every open outbound lane at zero.

### Population-aware portal routing rollout

Release `0.3.6` changes the pre-custody overload path. A live destination that explicitly refuses
a migration no longer receives the next offer merely because it remains live. The source records
that refusal durably and continues in the same direction to the next compatible world it has not
tried. Queue overload and the new population policy both use this custody-safe path; relay queue
pressure remains a retry to the same destination and is not misclassified as a world refusal.

The population controller ships to participants in `adaptive-shadow`. It measures a per-world
capacity estimate for ×10 and publishes the limit and open/closed decision without refusing on
population. Fixed and enforcing adaptive modes remain explicit operator choices. The normal
promotion gate requires at least 24 hours of stable shadow evidence and the other checks in
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

The service host then activated the archive and relay binaries built from `c321aad`. The guarded
archive restart replayed for 75 seconds; the peer gate made the participant outage 78 seconds.
The archive subscribed before any placement claim, so the receipt reports a complete record. At
the 60-second closeout, the map had the same 15 live and four dark slots and a connected relay.
All four public routes checked returned HTTP `200`. The public status reported 11 mod `0.6.8`
worlds in `adaptive-shadow`, zero enforcing population gates, and the live page carried the new
population-admission explanation. The announcements page published the planned window and its
actual return time.

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
| Network protocol in force | `contract-b/4.1`, which the current release speaks. An older world speaking `contract-b/4.0` still joins unchanged: every difference is optional |
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
| Release | [`v0.3.6`](https://github.com/jpinedaa/bibites-multiverse/releases/tag/v0.3.6) |
| Supported game | *The Bibites* `0.6.3.1` |
| Plugin | `0.6.8` |
| Mod-to-sidecar protocol | `contract-a/2.4` |
| Network protocol | `contract-b/4.1` — what this release speaks, and what the hosted service speaks; see [Hosted service](#hosted-service). A world on `contract-b/4.0` still joins |
| Windows package | Single setup executable with the authorized portable game, application shortcuts, and uninstall registration. An existing game is optional |
| Windows launcher | The shortcuts open `BibitesMultiverseLauncher.exe`, a window that lists every world on this computer with what is running, whether its mod has reached the map, its speed and its slot, and starts, stops, creates, clones, deletes and diagnoses them. A per-session switch runs one session with or without a window. The same launcher's commands and console menu ship beside it as `multiverse-launcher.exe`, which is what a script calls. One game folder supports five worlds at once |
| Linux package | Complete archive with the authorized native game; an existing-game add-on remains available |
| Public-map setup | Every participant package includes `public-map.json`. Installation creates a unique credential over HTTPS |
| Homepage | Links the Windows setup, the Linux complete package, and the checksums of the newest release, and names which release that is. Neither the links nor the number carry a compiled-in version — the links address `/releases/latest` and the number is resolved from GitHub hourly in the background — so a release reaches the homepage with no deployment, and the page drops the number rather than guess if that lookup fails. A three-step Windows walkthrough sits under those links, with one line for Linux |
| Live console | Full-screen map fitting, visible-range brain and population charts, all-time and 24-hour population views, shared navigation, and a live homepage status light are deployed. The Species tab expands vertically with the page. Wide timeline content keeps its horizontal scroll inside the drawing. The genealogy view separates reused names into immutable lineage instances and labels unresolved evidence. |
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
