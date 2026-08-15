# Project status

Last updated: 2026-08-15 UTC.

Bibites Multiverse `0.2.1` is public. The first announced service period runs from
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
| Release | [`v0.2.1`](https://github.com/jpinedaa/bibites-multiverse/releases/tag/v0.2.1) |
| Supported game | *The Bibites* `0.6.3.1` |
| Plugin | `0.6.4` |
| Mod-to-sidecar protocol | `contract-a/2.4` |
| Network protocol | `contract-b/4.0` |
| Windows package | GUI complete archive with the authorized portable game; an existing game is optional |
| Linux package | Complete archive with the authorized native game; an existing-game add-on remains available |
| Public-map setup | Every participant archive includes `public-map.json`. Installation creates a unique credential over HTTPS |
| Live console | Full-screen map fitting, visible-range brain charts, shared navigation, and a live homepage status light are deployed. |

The release page provides checksums. The participant installer compares the game build with
the support matrix before it changes the game directory. The Windows GUI starts the connected
world after installation by default and keeps headless mode in the generated start script.
The packaged public join configuration contains service addresses, not a shared world secret.
Release `0.2.1` adds a Linux complete package and automatic enrollment to the Linux installer.
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
- Open the [shared broadcast](https://bibitesmultiverse.com/watch) to watch one world.
- Use [GitHub issues](https://github.com/jpinedaa/bibites-multiverse/issues) for defects and
  experiment proposals.
