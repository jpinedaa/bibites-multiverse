<div align="center">

<img src="docs/assets/multiverse-hero.jpg" width="100%" alt="Five independent digital ecosystems linked by two-way organism migration paths">

# Bibites Multiverse

**Artificial life, evolving across the network.**

[![Installer status: 0.1.0 released](https://img.shields.io/badge/installer-0.1.0%20released-4ec9a0)](https://github.com/jpinedaa/bibites-multiverse/releases/tag/v0.1.0)
[![Windows](https://img.shields.io/badge/Windows-Steam-5aa9e6)](docs/participant/install.md)
[![Linux](https://img.shields.io/badge/Linux-native-4ec9a0)](docs/participant/install.md)
[![License](https://img.shields.io/badge/license-Apache--2.0-8b95a3)](LICENSE)

[Website](https://bibitesmultiverse.com/) ·
[Live map](https://bibitesmultiverse.com/live) ·
[Watch live](https://bibitesmultiverse.com/watch) ·
[Install](docs/participant/install.md) ·
[Join](docs/participant/join.md) ·
[Participant guide](docs/README.md) ·
[Project status](STATUS.md)

</div>

Bibites Multiverse connects independent copies of *The Bibites* as neighboring ecosystems.
Organisms cross one simulation border and continue their lives in another world.

Every participant keeps a separate game, clock, ecosystem, and save history. This is not one
synchronized mega-simulation. It is an evolutionary continent made from worlds that remain local.

> [!IMPORTANT]
> **Bibites Multiverse `0.1.0` is public.** Download the Windows or Linux add-on archive from the
> [release page](https://github.com/jpinedaa/bibites-multiverse/releases/tag/v0.1.0). These editions
> connect an existing game installation. Make sure that the SHA-256 matches before you extract it.
> Do not download a Multiverse build from another source.

## About *The Bibites*

[*The Bibites: Digital Life*](https://store.steampowered.com/app/2736860/The_Bibites_Digital_Life/)
is an artificial-life sandbox by Omnia Studios. Every Bibite inherits mutable genes and a
neural-network brain.

You set the physical and biological conditions of a world. Natural selection shapes the creatures
that live there. Their bodies, brains, behavior, and species can change across generations.

You can get *The Bibites* on [Steam](https://store.steampowered.com/app/2736860/The_Bibites_Digital_Life/)
or [itch.io](https://thebibites.itch.io/the-bibites).

These screenshots come from real Multiverse saves loaded in the game.

<table>
<tr>
<td width="50%">
<img src="docs/assets/gameplay-ecosystem.webp" alt="Several living Bibites sharing one green simulation world">
<br><sub><strong>A world that evolves itself.</strong> Food, movement, reproduction, predation, and mutation shape each local population.</sub>
</td>
<td width="50%">
<img src="docs/assets/gameplay-neural-network.webp" alt="A living Bibite beside its neural-network brain diagram">
<br><sub><strong>Brains evolve too.</strong> Each Bibite turns sensory inputs into actions through its own network of mutable nodes and connections.</sub>
</td>
</tr>
</table>

<img src="docs/assets/gameplay-species-lineage.webp" width="100%" alt="The Bibites ancestral lineage graph showing branches across 155 recorded species">

<p align="center"><sub><strong>Speciation leaves a history.</strong> This real save has 155 recorded species. The game shows when living branches split from their ancestors.</sub></p>

Each simulation is a living world. Connect many of them, and artificial life can inhabit a digital
universe larger than any one computer.

> **What evolves when the network itself becomes a habitat?**

Bibites Multiverse is an independent community project. It is not affiliated with or endorsed by
Omnia Studios.

## What the Multiverse adds

<img src="docs/assets/multiverse-map.svg" width="100%" alt="Three independent Bibites worlds connected by two-way migration lanes and a public archive">

Each player still runs a complete local simulation. The Multiverse turns the borders between
those simulations into migration routes.

| 🧬 Evolution can travel | 🗺️ Every world stays independent |
|---|---|
| A border crossing can become a migration into another ecosystem. | Your clock, settings, saves, and simulation remain on your machine. |
| **↔️ Every edge is a route** | **🌐 The map survives churn** |
| North, east, south, and west can send and receive organisms. | Offline worlds are bypassed. Returning worlds reclaim their prior positions. |
| **📚 History remains visible** | **🔐 Each world has one identity** |
| The public archive follows migrations, lineage, species, and brain-complexity trends. | Each participant gets a separate credential that binds to one world. |

The result feels less like a multiplayer lobby and more like a continent with moving life.

### The network in motion

<a href="https://bibitesmultiverse.com/live">
<img src="docs/assets/live-multiverse-map.webp" width="100%" alt="Six live Bibites worlds connected by migration lanes with organisms moving between them">
</a>

<p align="center"><sub><strong>One captured moment.</strong> The public map shows six live worlds, their populations, active lanes, and organisms in transit.</sub></p>

## Watch evolution unfold

### Species across the network

<a href="https://bibitesmultiverse.com/live#species">
<img src="docs/assets/live-species-view.webp" width="100%" alt="The public Species tab showing living species, their worlds, populations, trends, and recorded ancestry">
</a>

The [Species tab](https://bibitesmultiverse.com/live#species) joins live census reports with the
migration archive. It shows which species are alive, where they live, how their populations are
changing, and how complex their brains have become. It connects ancestry only when the migration
record has seen that lineage cross between worlds.

### One life at a time

<a href="https://bibitesmultiverse.com/watch">
<img src="docs/assets/live-broadcast-view.webp" width="100%" alt="A spectator-camera view following one living Bibite">
</a>

The [live broadcast](https://bibitesmultiverse.com/watch) gives every visitor the same read-only
camera. It selects the youngest living Bibite and stays with that creature until it dies or leaves
the world. Then it chooses another. The image above is a real spectator-camera view from a
Multiverse save.

## Install one world

Release `0.1.0` turns the player setup into four steps:

1. Get one private join string from the map operator.
2. Download the archive for your platform and make sure that its checksum matches.
3. Extract the archive and run its installer.
4. Run the generated start script to open the sidecar and your world.

After checksum and extraction, the platform commands are:

| Platform | Supported game | Install | Start |
|---|---|---|---|
| Windows | *The Bibites* 0.6.3.1 from Steam | Double-click `Install-BibitesMultiverse.cmd` | `.\Start-Multiverse.ps1` |
| Linux | *The Bibites* 0.6.3.1 from itch.io | `./install-bibites-multiverse.sh` | `./start-multiverse.sh` |

Use `.\Start-Multiverse.ps1 -Headless` or `./start-multiverse.sh --headless` to run a world
without graphics. The simulation remains active.

The installer finds the game and makes sure that its build is supported. It installs the mod,
stores the credential, and creates the start and stop scripts. It needs no compiler, SDK,
administrator account, or root access.

Release `0.1.0` contains add-on packages. The installer finds an existing game automatically.

[Read the full installation guide →](docs/participant/install.md)

## How it works

```mermaid
%%{init: {"theme":"base","fontFamily":"Arial, sans-serif","flowchart":{"nodeSpacing":35,"rankSpacing":40,"diagramPadding":8},"themeVariables":{"background":"transparent","primaryTextColor":"#f6f8fa","textColor":"#f6f8fa","lineColor":"#8b949e","edgeLabelBackground":"#0d1117"}}}%%
flowchart TB
    accTitle: How Bibites Multiverse connects independent simulations
    accDescr: A local game uses the mod and sidecar to exchange organisms through the public relay. The archive records events, while every world and save remains on its owner's computer.

    subgraph PLAYER["YOUR COMPUTER · PRIVATE"]
        direction TB
        subgraph GAME_PROCESS["THE BIBITES PROCESS"]
            direction LR
            Game["The Bibites<br/>simulation + local saves"]
            Mod["Multiverse mod<br/>detects border crossings"]
            Game <-->|"in-process"| Mod
        end
        Sidecar["Sidecar<br/>durable custody + reconnect"]
        Mod <-->|"localhost · Contract A"| Sidecar
    end

    subgraph SERVICE["MULTIVERSE SERVICE · PUBLIC"]
        direction LR
        Relay["Relay<br/>placement + routing"]
        Archive[("Archive<br/>live map + history")]
        Relay -.->|"records events"| Archive
    end

    subgraph PEERS["OTHER COMPUTERS · PRIVATE"]
        Neighbors["Neighbor worlds<br/>independent simulations + saves"]
    end

    Sidecar <-->|"encrypted WSS<br/>Contract B"| Relay
    Relay <-->|"routed migrants"| Neighbors

    classDef game fill:#16251f,stroke:#4ec9a0,stroke-width:2px,color:#f6f8fa
    classDef local fill:#102a30,stroke:#46c2c7,stroke-width:2px,color:#f6f8fa
    classDef service fill:#13253b,stroke:#5aa9e6,stroke-width:2px,color:#f6f8fa
    classDef archive fill:#251c36,stroke:#b18cff,stroke-width:2px,color:#f6f8fa
    classDef peer fill:#2b2417,stroke:#e7b65c,stroke-width:2px,color:#f6f8fa

    class Game game
    class Mod,Sidecar local
    class Relay service
    class Archive archive
    class Neighbors peer

    style PLAYER fill:#0d1117,stroke:#4ec9a0,stroke-width:1px,color:#f6f8fa
    style GAME_PROCESS fill:#111a17,stroke:#2d6a55,stroke-width:1px,color:#8b949e
    style SERVICE fill:#0d1117,stroke:#5aa9e6,stroke-width:1px,color:#f6f8fa
    style PEERS fill:#0d1117,stroke:#e7b65c,stroke-width:1px,color:#f6f8fa

    linkStyle 0,1 stroke:#46c2c7,stroke-width:2px
    linkStyle 2 stroke:#b18cff,stroke-width:2px
    linkStyle 3,4 stroke:#5aa9e6,stroke-width:3px
```

The mod detects border crossings. The sidecar records custody before it sends a migrant.
The relay assigns map positions and routes each migration. The archive records the public history.

Stable migration identities make retries safe. The design prefers a rare lost migration over a
duplicated organism because duplication changes the simulation permanently.

[Read the system design →](system_decomposition.md)

## Designed for players, not build machines

- The package includes the mod, BepInEx, the sidecar, and native platform scripts.
- The installer refuses unsupported game builds before it changes the game folder.
- The start script launches the sidecar before it launches the game.
- The stop script closes both processes without removing the world or its saves.
- The uninstaller keeps changed files and removes only files that its install record owns.
- Headless mode can run a world without graphics.

## Trust boundary

A connected world crosses a real network boundary. The package keeps that boundary explicit.

- Release archives carry SHA-256 checksums and an internal file manifest.
- TLS protects traffic between every sidecar and the relay.
- Join strings never belong in issue reports, screenshots, logs, or command lines.
- The sidecar accepts mod traffic only from the same computer.
- Your world and save files remain local.
- The participant guides state what the public service records and retains.

Read [what joining publishes](docs/participant/join.md#what-joining-publishes-about-your-world)
before you connect a world.

## Public experiment

The hosted entry point is [bibitesmultiverse.com](https://bibitesmultiverse.com/).
The first announced service period runs from **August 14 through November 14, 2026**.
Read the [project status](STATUS.md) for the release and milestone state.

The homepage explains the experiment and shows a live summary. The
[live console](https://bibitesmultiverse.com/live) shows the map, migrations, species,
lineages, and world settings.

The [watch page](https://bibitesmultiverse.com/watch) has one shared game camera.
It follows the youngest living Bibite until that Bibite dies or leaves the world.
The page reconnects automatically when the broadcast host is offline.

## Explore the experiment

| Goal | Start here |
|---|---|
| Watch one Bibite live | [Open the shared broadcast](https://bibitesmultiverse.com/watch) |
| Watch the public map and its evolutionary record | [Open the live map](https://bibitesmultiverse.com/live) |
| Connect a world | [Read the participant guide](docs/README.md) |
| Understand the architecture | [Read the system design](system_decomposition.md) |
| Inspect the network protocol | [Read Contract B](contracts/contract-b-m4.md) |
| Report a defect or propose an experiment | [Open a GitHub issue](https://github.com/jpinedaa/bibites-multiverse/issues) |

Bug reports, documentation corrections, and reproducible experiment ideas are welcome.

## Repository guide

| Path | Purpose |
|---|---|
| [`release/`](release/) | Player installers, uninstallers, release assembly, and package tests |
| [`bibites-mod/`](bibites-mod/) | BepInEx and Harmony game plugin |
| [`go/`](go/) | Sidecar, relay, archive, diagnostics, and protocol tests |
| [`docs/`](docs/) | Participant guides, support matrix, and operator diagnostics |
| [`cloud/aws/`](cloud/aws/) | Cloud-world and GPU-broadcast deployment |
| [`contracts/`](contracts/) | Versioned mod and network protocols |
| [`deploy/`](deploy/) | Public service provisioning, monitoring, backup, and recovery |
| [`e2e/`](e2e/) | Multi-world test rigs and failure rehearsals |

Developer entry points:

- [System architecture](system_decomposition.md)
- [Live broadcast design and operations](docs/live-broadcast.md)
- [Developer guide and technical findings](dev_environment.md)
- [Release engineering](release/README.md)
- [Mod-to-sidecar protocol](contracts/contract-a.md)
- [Sidecar-to-relay protocol](contracts/contract-b-m4.md)

## License

Original Bibites Multiverse work uses the [Apache License 2.0](LICENSE). Third-party components
and game payloads keep their own terms. See the [third-party notices](THIRD_PARTY_NOTICES.md).
