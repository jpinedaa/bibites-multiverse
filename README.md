<div align="center">

# Bibites Multiverse

**One simulation is a world. Connect them, and evolution gets a map.**

Bibites Multiverse connects separate copies of *The Bibites* as neighboring ecosystems.
Organisms cross the edge of one simulation and enter the next.
Every participant still owns, runs, and saves a complete local world.

[Releases](https://github.com/jpinedaa/bibites-multiverse/releases) ·
[Install](docs/participant/install.md) ·
[Join the map](docs/participant/join.md) ·
[Participant guide](docs/README.md)

</div>

---

## The edge is no longer the end

In a normal simulation, the border is final. In the Multiverse, every border can become a migration route.

```text
                 north
                   ↕
     west  ↔  YOUR WORLD  ↔  east
                   ↕
                 south
```

A Bibite that crosses east can arrive in another person's world. Their climate, food, predators, and selection pressures take over from there.

Their organisms can cross into your world too. Evolution remains local, but its consequences can travel.

## One map, many independent worlds

This project does not create one synchronized mega-simulation. Each participant controls a separate copy of *The Bibites*.

- Your world keeps its own clock, speed, settings, and save files.
- All four edges can carry organisms in both directions.
- The map routes around worlds that go offline.
- Returning participants reclaim their previous place.
- The public archive follows migrations, lineage, species, and brain-complexity trends.

The result feels less like a multiplayer lobby and more like an evolutionary continent.

## How it works

```text
The Bibites
    ↕  BepInEx + Harmony
Multiverse mod
    ↕  local WebSocket
Participant sidecar
    ↕  encrypted connection
Central relay
    ↕
Neighboring worlds + public archive
```

The mod watches the simulation borders. The sidecar keeps durable custody of migrants and communicates with the relay.
The relay assigns map positions, routes migrations, and records public history.

Journaled handoffs and stable migration identities make retries safe. The system prefers a rare lost migration over a duplicated organism.

For the complete design, read the [system decomposition](system_decomposition.md) and the [protocol contracts](contracts/).

## Join the map

Bibites Multiverse supports *The Bibites* 0.6.3.1 on these platforms:

| Platform | Game distribution |
| --- | --- |
| Windows | Steam |
| Native Linux | itch.io |

1. Choose an edition on [GitHub Releases](https://github.com/jpinedaa/bibites-multiverse/releases).
   Use the add-on edition with an existing game installation. Use a published complete edition
   to install the game and Multiverse together.
2. Download its archive and SHA-256 checksum.
3. Follow the [installation guide](docs/participant/install.md) for your platform.
4. Use the [join guide](docs/participant/join.md) to connect your world.

The installers require no compiler, SDK, administrator account, or root access.

If something goes wrong, start with the [diagnostic guide](docs/participant/diagnose.md). To disconnect cleanly, follow the [leave guide](docs/participant/leave.md).

## Safety by design

A connected simulation crosses a real trust boundary. The project makes that boundary visible.

- Release archives include SHA-256 checksums.
- TLS protects traffic between each sidecar and the relay.
- Every participant receives separate credentials.
- Secrets do not appear in process command lines.
- The sidecar accepts local mod traffic only from the same machine.
- Join and leave procedures state what the service publishes and retains.

Read the [participant guide](docs/README.md) before connecting a world.

## A map with an ending

The first public map is a bounded experiment, not a promise of permanent hosting.
Its published lifecycle includes advance reminders, a final map capture, and documented cleanup.

Participants can leave at any time. The [leave guide](docs/participant/leave.md) explains local cleanup, credential revocation, and data expectations.

## Build the Multiverse

This repository contains the game mod, participant sidecar, central relay, release tooling, protocol contracts, and operating documentation.

Useful starting points:

- [System architecture](system_decomposition.md)
- [Participant support surface](docs/README.md)
- [Mod-to-sidecar protocol](contracts/contract-a.md)
- [Sidecar-to-relay protocol](contracts/contract-b-m4.md)
- [Release engineering](release/README.md)

The project values observable behavior, explicit trust boundaries, and recovery that participants can understand.

## About *The Bibites*

*The Bibites* is an artificial-life simulation available from its creator on
[Steam](https://store.steampowered.com/app/2736860/The_Bibites_Digital_Life/) and
[itch.io](https://thebibites.itch.io/the-bibites).

Bibites Multiverse is an independent community project. It is not affiliated with or endorsed by *The Bibites*.

## License

Original Bibites Multiverse work is available under the [Apache License 2.0](LICENSE).
Third-party components and game payloads retain their own terms. See the [third-party notices](THIRD_PARTY_NOTICES.md).
