# Project status

Last updated: 2026-08-17 UTC.

Bibites Multiverse `0.3.0` is public. The first announced service period runs from
**August 14 through November 14, 2026**, and reminders begin 30 days before the end.

## Current public phase

M5 delivered the public release, hosted service, participant packages, and support guides.
The public experiment now collects the evidence for the M5 exit test.

The exit test requires all of these results:

- At least four participants who are not the project owner.
- At least 72 hours of continuous operation.
- No operator action on a participant's computer.

This page does not report live service health, peer counts, or join-string availability.
Use the [live map](https://bibitesmultiverse.com/live) for the public view of the experiment.

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
| Release | [`v0.3.0`](https://github.com/jpinedaa/bibites-multiverse/releases/tag/v0.3.0) |
| Supported game | *The Bibites* `0.6.3.1` |
| Plugin | `0.6.7` |
| Mod-to-sidecar protocol | `contract-a/2.4` |
| Network protocol | `contract-b/4.1` — what this release speaks, and what the hosted service speaks; see [Hosted service](#hosted-service). A world on `contract-b/4.0` still joins |
| Windows package | Single setup executable with the authorized portable game, application shortcuts, and uninstall registration. An existing game is optional |
| Windows launcher | The shortcuts open `BibitesMultiverseLauncher.exe`, a window that lists every world on this computer with what is running, whether its mod has reached the map, its speed and its slot, and starts, stops, creates, clones, deletes and diagnoses them. A per-session switch runs one session with or without a window. The same launcher's commands and console menu ship beside it as `multiverse-launcher.exe`, which is what a script calls. One game folder supports five worlds at once |
| Linux package | Complete archive with the authorized native game; an existing-game add-on remains available |
| Public-map setup | Every participant package includes `public-map.json`. Installation creates a unique credential over HTTPS |
| Homepage | Links the Windows setup, the Linux complete package, and the checksums of the newest release, and names which release that is. Neither the links nor the number carry a compiled-in version — the links address `/releases/latest` and the number is resolved from GitHub hourly in the background — so a release reaches the homepage with no deployment, and the page drops the number rather than guess if that lookup fails. A five-step Windows walkthrough sits under those links, with one line for Linux |
| Live console | Full-screen map fitting, visible-range brain and population charts, all-time and 24-hour population views, shared navigation, and a live homepage status light are deployed. |
| Broadcast world | The broadcast page names the world on camera and draws its place in the map grid. The live map badges that world on its map cell, in the worlds table, and in its settings card, and every badge links back to the broadcast. |
| Release process | Every pull request and every push to `main` runs the project's automated checks, and a version tag builds and publishes the release from the owner's own machine. The homepage follows the published release on its own |

The release page provides checksums. The participant installer compares the game build with
the support matrix before it changes the game directory, and it refuses an install over a world
that is still running before it has changed anything. The Windows GUI starts the connected
world after installation by default. A world starts at ten times the game's own speed, which the
in-game speed slider still moves. The launcher runs a world headless, and a stop asks the world
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
Private maps still accept a private join-string file on both platforms.

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
- Read the [announcements page](https://bibitesmultiverse.com/announcements/) for service notices.
  It is the only channel the service publishes them on.
- The optional [shared broadcast](https://bibitesmultiverse.com/watch) follows one participant
  world on the map. It is relevant only when a publisher is available.
- Use [GitHub issues](https://github.com/jpinedaa/bibites-multiverse/issues) for defects and
  experiment proposals.
