# Project status

Last updated: 2026-08-17 UTC.

Bibites Multiverse `0.2.7` is public. The first announced service period runs from
**August 14 through November 14, 2026**.

## Current public phase

M5 delivered the public release, hosted service, participant packages, and support guides.
The public experiment now collects the evidence for the M5 exit test.

The exit test requires all of these results:

- At least four participants who are not the project owner.
- At least 72 hours of continuous operation.
- No operator action on a participant's computer.

This page does not report live service health, peer counts, or join-string availability.
Use the [live map](https://bibitesmultiverse.com/live) for the public view of the experiment.

## Public release

| Item | Public state |
|---|---|
| Release | [`v0.2.7`](https://github.com/jpinedaa/bibites-multiverse/releases/tag/v0.2.7) |
| Supported game | *The Bibites* `0.6.3.1` |
| Plugin | `0.6.5` |
| Mod-to-sidecar protocol | `contract-a/2.4` |
| Network protocol | `contract-b/4.0` |
| Windows package | Single setup executable with the authorized portable game, application shortcuts, and uninstall registration. An existing game is optional |
| Windows launcher | The shortcuts open `BibitesMultiverseLauncher.exe`. It starts, stops, and reports each world, runs a world headless, and manages more than one world on one computer. One game folder supports five worlds at once |
| Linux package | Complete archive with the authorized native game; an existing-game add-on remains available |
| Public-map setup | Every participant package includes `public-map.json`. Installation creates a unique credential over HTTPS |
| Homepage | Links the Windows setup, the Linux complete package, and the checksums of the newest release. The links carry no version, so a release reaches the homepage with no deployment |
| Live console | Full-screen map fitting, visible-range brain and population charts, all-time and 24-hour population views, shared navigation, and a live homepage status light are deployed. |
| Broadcast world | The broadcast page names the world on camera and draws its place in the map grid. The live map badges that world on its map cell, in the worlds table, and in its settings card, and every badge links back to the broadcast. |
| Release process | Every pull request and every push to `main` runs the project's automated checks, and a version tag builds and publishes the release from the owner's own machine. The homepage follows the published release on its own |

The release page provides checksums. The participant installer compares the game build with
the support matrix before it changes the game directory. The Windows GUI starts the connected
world after installation by default. The launcher runs a world headless, and the generated
start script keeps its own headless switch.
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
- The optional [shared broadcast](https://bibitesmultiverse.com/watch) follows one participant
  world on the map. It is relevant only when a publisher is available.
- Use [GitHub issues](https://github.com/jpinedaa/bibites-multiverse/issues) for defects and
  experiment proposals.
