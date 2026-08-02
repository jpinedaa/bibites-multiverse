Distributed Simulation Architectures in The Bibites: Cross-Instance Entity Migration and Network Handoff Protocols
The pursuit of open-ended artificial life (ALife) simulations inherently encounters strict computational ceilings when confined to the processing power and memory space of a single monolithic hardware instance. In sophisticated ecological simulations such as The Bibites, where neural network topologies, kinematic physics calculations, and complex environmental interactions scale quadratically with population density, the concept of distributed computing represents the definitive frontier of ALife architecture. The transition from isolated sandbox environments into a cohesive, distributed "multiverse"—where distinct game instances dynamically exchange digital organisms over a network—aligns with broader academic pursuits in distributed dynamical systems and self-organizing intelligent matter.   

This exhaustive architectural report evaluates the community-driven efforts, existing modding frameworks, reverse-engineered data schemas, and theoretical implementation models required to establish cross-instance entity migration and multi-node server environments for The Bibites. Through a rigorous synthesis of community repositories, developer communications, and runtime execution hooks, this document provides a comprehensive blueprint for programmatically serializing, transmitting, and injecting live simulation entities across network sockets without requiring underlying engine source access.

1. Community Discourse and Developer Roadmaps
The desire to transition the simulation from an isolated local executable into a distributed, multi-node ecosystem has generated extensive technical discourse within community channels, particularly on the official Discord and the r/TheBibites subreddit. Analyzing these historical threads and developer responses provides necessary context regarding the current limitations and the trajectory of official networking support.

1.1. Feature Requests for Dedicated Servers and Remote Viewers
Community requests have historically spanned from simple asynchronous export/import mechanics to live, synchronous multiplayer architectures. Early community proposals envisioned traditional multiplayer environments where users could host dedicated servers, allowing independently evolved organisms to coexist, compete, and interbreed in a shared geographic space.   

More sophisticated architectural requests have emerged as the community's understanding of the engine's limitations has grown. Active modders and technical users have repeatedly called for a decoupled "Dedicated Server / Remote Viewer" topology. This paradigm suggests running the core simulation logic headlessly on a remote server while allowing client applications to connect exclusively as visualizers. Such an approach would enable continuous, long-running evolutionary experiments (lasting thousands of hours) without binding a local machine's rendering thread or requiring the host machine to maintain an active GUI display. Furthermore, this architecture would allow multiple viewers to connect to a single public simulation simultaneously, effectively creating a persistent digital terrarium.   

1.2. Developer Stance on Multiplayer and Server Architectures
The primary developer, Léo Caussan, alongside the core development team, has publicly addressed these networking ambitions, providing a pragmatic assessment of the engine's current capabilities. During a Steam Next Fest Q&A session, the development team explicitly addressed the persistent community demand for distributed computing and server-hosted instances.   

The developers noted that running the simulation on a centralized server, paired with a client-side portal application to view the simulation remotely, is a potential long-term goal, though they cautioned that such an implementation resides on a hypothetical ten-year time horizon. Currently, the simulation is heavily bound by local processor execution logic, meaning that deploying the existing architecture to a traditional server without a fundamental engine refactoring would yield suboptimal performance. The current client is not designed with a decoupled client-server state reconciliation model, meaning that official distributed multiplayer support is not imminent. Consequently, the burden of achieving distributed execution and cross-instance migration falls entirely upon the community's modding ecosystem, which operates largely through the unofficial "Bibites Research Discord Server".   

1.3. The Asynchronous Baseline
In the absence of live network sockets, the community has normalized asynchronous, file-based entity exchange. The introduction of the Steam Workshop in update v0.6.2 provided a centralized repository for users to upload and download specific species, scenarios, and challenge saves. However, this official integration relies on static, manual uploads rather than automated programmatic handoffs.   

The community's GitHub repositories, such as Bibites_Shared_Content, serve as manual directories for sharing genetic data and logging notable evolutionary milestones. While these directories demonstrate the community's active interest in cross-pollinating genetic pools, they represent a high-friction process. For a true multi-node ecosystem, this file-based exchange must be abstracted into a runtime network protocol capable of executing these transfers dynamically.   

2. Existing Mods and Toolchains
To achieve seamless, programmatic entity export and import without requiring manual user intervention, deep hooks must be established within the core simulation logic. The community has developed several functional mods and external parsing tools that successfully touch upon entity serialization, runtime memory manipulation, and data schema extraction. These tools provide the foundational building blocks for a network protocol.

2.1. Functional and Abandoned Tooling
The following table categorizes the primary mods and tools that have successfully interacted with the game's internal data structures, establishing the feasibility of programmatic extraction and injection.

Tool / Mod Name	Primary Functionality	Relevance to Network Migration
Bibitinator	
A C# forms application that parses and edits .bb8 save files, featuring a tree-view mapping system for analyzing neural connections.

Demonstrates the precise logic required to deserialize the .bb8 schema, validate internal genetic parameters, and reconstruct valid JSON payloads for game engine ingestion.

EinsteinEditor	
A highly advanced offline visual editor for constructing and simulating small, complex brain topologies. Features auto-conversion between different bibite versions.

Introduces robust regex parsing ("^[0-9]+(\\.[0-9]+)(\\.[0-9]+)?([ aA][0-9]+)?$") to handle version control anomalies, a critical requirement for ensuring parity across a distributed multi-node network.

BibitesModsLoader	
A lightweight mod loader designed to inject standardized .dll payloads into the game environment.

Establishes an IMod interface and custom attributes (e.g., [Mod("NetworkGateway", "1.0.0", "Author")]) that could serve as the distribution vehicle for a standardized networking API.

Constance-Mod	
A runtime modification that locks specific genetic traits, neurons, and synapses to prevent mutational drift, actively intercepting the instantiation phase of newborn entities.

Represents the ultimate proof-of-concept for dynamic state injection. Demonstrates how to overwrite active memory state to align with an external template and how to manipulate live runtime attributes during the spawn loop.

  
2.2. The Constance-Mod: Runtime Injection Proof-of-Concept
The Constance-Mod, developed by YBKy, serves as the most comprehensive technical reference for manipulating Bibite data during runtime. While its primary function is genetic stabilization, its underlying mechanics demonstrate exactly how an entity's data can be intercepted and overwritten.   

The mod operates by targeting a specific tag within a configured .bb8 file placed in the [your Bibite folder]\The Bibites_Data\Mods directory. When a new organism hatches or is spawned, the mod intercepts the initialization routine within BibitesAssembly.dll. If the spawned entity's traits differ from the master .bb8 file, the mod dynamically overwrites the active memory state to align with the template.   

Crucially, the Constance-Mod implements a "Dummy Tag Mod" functionality that directly manipulates live attributes rather than just static genetic blueprints. By parsing the organism's tag for the string .dummy, the mod can freeze or modify dynamic runtime variables such as:

Stomach contents and current energy levels

Current health and dimensional scaling (d2Size)

Maturity and total time alive

Immortality states (setting dying to false)   

This precise runtime manipulation of live attributes is the exact mechanical requirement for cross-instance migration. When an organism arrives from a foreign server via a network socket, it cannot spawn as an infant. The receiving instance must instantiate a virgin organism, and then a Harmony patch must immediately overwrite its live attributes (health, stomach, energy) with the values provided in the JSON network payload. This ensures the organism resumes its life seamlessly, retaining its exact temporal state upon arriving in the new instance.

3. Entity Serialization and The .bb8 Data Schema
The foundational requirement for any network handoff is the ability to encapsulate the complete state of a digital organism, transmit it, and reconstruct it perfectly on a foreign instance. The Bibites utilizes a highly structured JSON schema denoted by the .bb8 file extension for entity serialization. The game engine relies on Newtonsoft.Json.UnityConverters.dll to manage the serialization and deserialization of these complex object graphs, alongside the core logic in BibitesAssembly.dll.   

Understanding this schema is critical, as it constitutes the exact data payload that must be transmitted over a network socket during a migration event. A single malformed integer index or missing boolean flag will result in a deserialization crash on the receiving server.   

3.1. Parsing the Physical Stats (Genes)
The first block of the .bb8 payload contains the physical stats, often referred to as genes. This is a relatively flat list of scalar values defining base physical attributes. These attributes control macro-level behaviors and physical limitations, such as diet type, view angle, base metabolism, and grab strength. Because this is a lightweight array of standard floating-point numbers, it is easily validated during a server handoff. A distributed network node can easily implement safety checks against this array to ensure parameters do not exceed bounds, preventing corrupted or artificially manipulated values from crashing the simulation.   

3.2. Mapping the Nodes (Neurons)
The brain of the organism is mapped sequentially in the JSON payload. All bibites inherently possess every non-hidden node available in the game; these nodes simply remain dormant until a synapse connects them. The default unmodded bibite possesses 48 primary nodes—comprising 33 inputs (senses) and 15 outputs (actions).   

Each node is encased in a JSON object block containing three primary identifiers:

TypeName: A string defining the mathematical activation function of the node (e.g., ReLu, TanH, Linear). This dictates how the node computes passing information.   

Index: The integer identifier crucial for mapping. For instance, the Constant node is always index 0, while the ImmuneSystem node operates as the final standard index before hidden nodes are appended.   

Desc: A human-readable description string (e.g., MeatConcentrationAngle) that appears in the simulation's UI.   

When transmitting an entity across a network, the receiving server must validate the node indices to ensure they map correctly to the engine's internal array. If the sending server utilizes mods that inject custom nodes—such as the Senses Plus mod (which adds nodes for Rotation Speed, Target Diet, and Target Size) or the YBKs Vision Rework Mod—and the receiving server does not have these mods installed, the Index array will misalign, causing severe instability or total engine failure.   

3.3. Reconstructing Synapses (Connections)
The most fragile aspect of the network payload is the synapse array, which represents the active topological map of the neural network. A virgin bibite contains no synapses, meaning this array is entirely empty upon birth.   

Each synapse object in the JSON defines a highly specific connection between two nodes:

NodeIn: The integer index of the input node.   

NodeOut: The integer index of the output node.   

Weight: A floating-point multiplier that alters the activation strength of the signal passing through the synapse. Modders have discovered that this weight can dynamically reference genes if passed as a string integer (e.g., "14"), allowing complex color-recognition logic.   

En: A boolean flag (true/false) determining if the synapse is actively transmitting data or is functionally dormant.   

Network transmission logic must ensure absolute fidelity in this array. Notably, connecting an input node to an input node, or an output to an output, violates engine constraints. While local manual edits using Notepad can introduce these fatal errors, a programmatic network exporter must sanitize the NodeIn and NodeOut indices before transmission to guarantee simulation stability on the target machine.   

4. Modding APIs and Runtime Serialization Hooks
To automate the extraction of the .bb8 schema and transmit it to a network socket, deep execution hooks must be established within the core simulation logic. The community has standardized around BepInEx and HarmonyX for runtime patching of BibitesAssembly.dll.   

4.1. BepInEx and HarmonyX Architecture
BepInEx acts as the core plugin loader and environment manager, while HarmonyX provides the specific capability to inject custom instructions into pre-compiled Intermediate Language (IL) code at runtime. By utilizing [HarmonyPatch] attributes, developers can define [HarmonyPrefix] and [HarmonyPostfix] methods that execute precisely before or after the simulation's native functions.   

The installation and project setup for a BepInEx plugin requires referencing the target Unity game libraries. A .NET Standard C# project must be created, and the .csproj file must be modified to include <HintPath> references to BibitesAssembly.dll and other necessary Managed libraries.   

The patching architecture follows a strict structural paradigm to intercept classes such as SimulationScripts.BibiteScripts.Bibite.   

C#
using HarmonyLib;
using System;

namespace NetworkGatewayPlugin
{
    [HarmonyPatch(typeof(SimulationScripts.BibiteScripts.Bibite))]
    internal static class NetworkMigrationPatch
    {
        [HarmonyPatch("Update")]
        [HarmonyPrefix]
        private static void Prefix(SimulationScripts.BibiteScripts.Bibite __instance)
        {
            // Spatial coordinate validation for border collision
            if (__instance.transform.position.x > GlobalNetworkConfig.BorderMaxX)
            {
                // Trigger Serialization Pipeline
            }
        }
    }
}
This structural capability is the linchpin for distributed networking because it allows a mod to intercept the Update() loop of every active organism on a frame-by-frame basis. If an organism meets the criteria for migration—such as touching a designated map border—the prefix patch can execute its state capture. Furthermore, if the developer wishes to override a native function entirely—such as preventing an organism from rendering or burning energy locally while it is in the process of being buffered over the network—they can instruct the prefix to return false;, effectively bypassing the original native method execution entirely.   

4.2. Programmatic Deserialization and Memory Overwrite
When receiving a payload from the network, a corresponding [HarmonyPostfix] must be utilized. Because Unity's internal engine architecture strictly prohibits the instantiation of GameObjects on background threads, the network socket must push incoming JSON payloads into a thread-safe concurrent queue.

During the main thread's FixedUpdate() cycle, a manager class dequeues the payload and invokes the native spawning functions within SimulationScripts. However, to prevent the engine from randomizing the newly spawned entity, a postfix hook on the spawning function intercepts the resulting object. Borrowing the logic pioneered by the Constance-Mod, this postfix forces an immediate overwrite of the entity's memory state, parsing the .bb8 string via Newtonsoft.Json and applying the exact neural topologies, gene values, and live attributes (e.g., stomach volume) defined in the network payload.   

5. Portal, Gateway, and Border Systems
In a distributed multi-node network, spatial continuity must be maintained to create the illusion of a continuous ecosystem. If Server Node A simulates the northern hemisphere of a digital world and Server Node B simulates the southern hemisphere, there must be a defined geographic boundary that triggers the network handoff of an entity.

5.1. Island Migration and Native Boundaries
Within the core game, the developer has implemented highly specific spatial boundary logic. Historically, the simulation boundaries consisted of a static, lethal void. This led to the implementation of "Automatic Void Avoidance" settings, wherein organisms possess an innate mathematical repulsion that prevents them from crossing the threshold.   

However, in update v0.5.0, this void-avoidance mechanic was significantly reworked to be "island-based," introducing localized zones that dictate where organisms can physically traverse. Since this update, players have actively observed and documented organic "island migration" behaviors. Reports on the subreddit indicate that organisms that achieve a certain maturity and energy threshold will occasionally override their void avoidance and cross the empty spaces bridging distinct geographic islands, especially when environmental pressures such as food scarcity dictate.   

This demonstrates that organisms are naturally inclined to explore peripheral boundaries. A network mod can capitalize on this existing behavior; rather than dying in the void between islands, venturing past an island boundary triggers the network export hook.

5.2. Teleportation and Wrap-Around Borders
Discussions surrounding map boundaries naturally extend to teleportation concepts. A conceptual mechanic discussed within the developer's itch.io comment sections involves altering the border crossing logic to simulate a spherical globe: instead of organisms hitting a physical wall or dying in the void, crossing the eastern edge of the simulation bounds would instantly teleport the organism to the western edge, preserving its velocity vector.   

This wrap-around teleportation logic maps perfectly to a distributed server architecture. By overriding the geographic coordinate check, a BepInEx mod can redirect the entity to a network socket instead of wrapping it locally.

5.3. Gateway Towers and Pheromone Portals
An alternative to edge-of-map boundaries is the implementation of localized "Gateway Portals." The game natively supports customizable zones that alter local physics (e.g., drag) and physical towers that emit environmental effects. For example, the Rad Tower emits a zone of radiation that increases mutational intensity, while the Pheromone Tower produces specific chemical gradients.   

A custom "Gateway Tower" could be implemented by extending the existing tower classes. This structure would emit a unique, mod-defined pheromone signature that attracts specific organisms (perhaps those evolved to seek out new territories). Upon an organism colliding with the tower's physical radius, the [HarmonyPrefix] hook triggers, serializing the organism and transmitting it to a central database or a peer-to-peer node. This acts as a physical, centralized portal rather than relying on invisible map borders, offering precise control over migration rates.

6. Implementation Feasibility: The Multi-Node Architecture Blueprint
Based on the capabilities of BepInEx, the .bb8 serialization format, and standard C# networking libraries available within the Unity Mono runtime, constructing a fully distributed "multiverse" network is demonstrably feasible. The following outlines a theoretical, end-to-end architectural blueprint for implementing programmatic cross-instance migration.

6.1. The Relay Server Topology
Due to the lack of native headless server support and the high CPU overhead of the simulation, a purely decentralized peer-to-peer mesh network is likely unviable. Instead, the architecture must rely on a hybrid Relay Topology.   

A lightweight, external Relay Server (written in Node.js, Golang, or an external C# application) acts as the central spatial router. Players run full instances of The Bibites on their local machines, acting as individual "compute nodes." A BepInEx network mod installed on each client establishes a persistent WebSockets or TCP connection to the Relay Server. The Relay Server maintains a global spatial map of the distributed universe, assigning specific grid coordinates (e.g., Sector 1A, Sector 1B) to each connected client instance.

6.2. The Export Pipeline (Serialization and Handoff)
When an organism touches a designated boundary or portal zone, the local mod executes the export pipeline:

Intercepting the Loop: A [HarmonyPrefix] on the Bibite.Update() method detects that the entity's spatial coordinates exceed the bounding box of the local sector.

Suppressing Native Logic: The prefix returns false to halt native processing, instantly freezing the organism's physics and neural updates.

Programmatic Serialization: The mod utilizes Newtonsoft.Json to serialize the entity into a .bb8 string. It specifically appends the live runtime attributes (health, energy, stomach contents, maturity) to the payload, utilizing the access paradigms proven by the Constance-Mod.   

Network Transmission: The JSON payload is wrapped in a transport protocol wrapper containing destination metadata (e.g., "Target Node: Sector 1B", "Entry Coordinates: X, Y", "Velocity Vector: V") and pushed via a non-blocking TCP socket to the Relay Server.

Local Destruction: Once the Relay Server acknowledges receipt of the full payload, the local mod safely purges the entity from memory using UnityEngine.Object.Destroy().

6.3. The Import Pipeline (Deserialization and Injection)
On the receiving client instance (Sector 1B), the import pipeline reconstructs the entity without interrupting the local simulation frame rate.

Socket Listener: A background thread managed by the BepInEx plugin continuously monitors the TCP socket. When a payload arrives from the Relay Server, it is parsed and pushed into a concurrent queue.

Main Thread Synchronization: A custom MonoBehaviour injected into the scene polls the network queue during the native FixedUpdate() cycle.

Entity Instantiation: The mod invokes the native SimulationScripts spawning methods.

State Injection: A [HarmonyPostfix] intercepts the spawned object, executing an immediate state overwrite. The .bb8 payload is deserialized, mapping the correct nodes and synapses. The live attributes are forcibly injected, setting the entity's stomach contents and energy levels to match the exact values it held when it departed the previous node.

Coordinate Placement: The entity's transform.position is set to the appropriate geographic entry point (e.g., spawning on the western edge because it exited the eastern edge of the previous sector), and its velocity vector is restored.

6.4. Environmental Continuity: Handling Corpses and Inert Mass
For a truly cohesive distributed ecosystem, environmental continuity must extend beyond living organisms. The introduction of the Corpses system in v0.6.3 adds a critical layer to the simulation's metabolic cycle. When an organism's health reaches zero, it turns into a corpse—a brain-dead entity that slowly decays based on its previous metabolism, eventually exploding into scavengable meat pellets.   

If an organism dies near a spatial boundary, its corpse or the resulting meat pellets may be pushed across the border by kinetic collisions from living bibites. The network mod must extend its boundary serialization hooks to include generic Pellet and Corpse objects. The serialization schema for a corpse is significantly simpler than a live organism, as it lacks a neural network topology. The mod simply transmits the object's mass, material type, and decay state across the network socket, ensuring that biomass is not artificially lost or trapped at the edges of server nodes.

6.5. Systemic Challenges and Mitigation Strategies
While this blueprint is mechanically sound based on the available tooling, a distributed architecture faces several systemic challenges due to the game's deterministic physics engine and the lack of native client-server reconciliation logic.

Challenge	Technical Description	Proposed Mitigation Strategy
Entity ID Collisions	The simulation assigns unique internal integer IDs to all active entities. Merging organisms from distinct simulations risks ID overlap, leading to memory access violations.	The network mod must intercept the incoming payload and dynamically re-index the organism's internal IDs to match the host instance's current registry sequence prior to instantiation.
Neural Network Discrepancies	
If Server A utilizes a heavily modded client with custom activation nodes (e.g., the Neurons Plus mod), and Server B runs vanilla, transmitting an organism will result in a fatal deserialization failure.

The Relay Server must enforce strict version and mod-list parity among connected nodes. It can utilize version regex parsing strings upon initial handshake, rejecting connections that do not match the global standard.

Boundary Jitter and Latency	If an organism's behavior causes it to oscillate rapidly across a mathematical border, it could trigger hundreds of continuous network serializations per second, overwhelming the socket and stalling both clients.	The implementation of a hysteresis loop (a spatial cooldown zone) is mandatory. Once an organism crosses a boundary and spawns on the new server, a temporary repulsion force or logic flag prevents it from migrating back for a specified time interval or spatial distance.
  
7. Conclusions
The transition of The Bibites from a strictly localized application into a distributed, multi-node network represents a highly complex, yet fundamentally solvable, software engineering challenge. While the core development team has indicated that official headless server support and decoupled remote viewers reside on a distant, hypothetical roadmap, the robust modding infrastructure established by the community provides all the necessary architectural components to construct an unofficial distributed gateway today.   

The standardization of BepInEx and HarmonyX within the modding community allows for surgical, runtime interception of the game's execution loops, enabling developers to pause logic, overwrite variables, and bypass native methods without requiring engine source access. The structural transparency of the .bb8 JSON schema—deeply reverse-engineered by tools like Bibitinator and EinsteinEditor—ensures that genetic templates, complex neural network topologies, and dynamic runtime attributes can be accurately captured, parsed, and reconstructed. Furthermore, the runtime state-injection paradigms proven by the Constance-Mod validate the feasibility of forcibly applying network payloads to newly instantiated entities without breaking engine stability.   

By combining these proven methodologies, a comprehensive network protocol can be established. Whether utilizing native island boundaries or custom Pheromone Gateway Towers to serve as physical network sockets, the community possesses the capability to seamlessly serialize an organism on one client, transmit its JSON payload via a relay server, and inject it into the simulation loop of another. This architecture effectively bridges the gap between isolated computational sandbox environments and a scalable, decentralized artificial life multiverse, enabling unprecedented collaborative research into the mechanics of open-ended evolution and distributed dynamical systems.   


2025.alife.org
ALife 2025 Program - Artificial Life
Opens in a new window

reddit.com
Export/import (feature request) : r/TheBibites - Reddit
Opens in a new window

reddit.com
Dedicated Server/Remote Viewer : r/TheBibites - Reddit
Opens in a new window

reddit.com
Every Question and Answer in the Steam Next Fest Bibites Q&A (and some highlights) : r/TheBibites - Reddit
Opens in a new window

reddit.com
Since I discovered the Bibites I am wondering: COULD THERE BE A SERVER WITH A CONTINIOUSLY RUNNING SIM? : r/TheBibites - Reddit
Opens in a new window

reddit.com
Bibites Research Discord Server! The Official Fan Discord Server, with collaborative research, external tools (mods, editors, analysis tools) and bibite sharing! Link in comments :) : r/TheBibites - Reddit
Opens in a new window

reddit.com
Bibites Research Discord Server! The Official Bibites Fan Discord Server, with collaborative research, external tools (mods, bibite editors, gene analysis tools) and bibite sharing! (link in comments) : r/TheBibites - Reddit
Opens in a new window

steamdb.info
The Bibites 0.6.2: Steam Workshop · The Bibites: Digital Life update for 25 June 2025 - SteamDB
Opens in a new window

github.com
GitHub - TheBibites/Bibites_Shared_Content: A directory for sharing and logging notable Bibite content for the community
Opens in a new window

github.com
TheBibites - GitHub
Opens in a new window

github.com
GitHub - JustinBrownDev/Bibitinator: This project aims to create a comprehensive file editor for the evolution simulator "The Bibites". Including editing the neural networks and genes of individual bibites and editing the simulation parameters of a saved game.
Opens in a new window

the-bibites.fandom.com
Editing Bibites (Tutorial) - The Bibites Wiki - Fandom
Opens in a new window

github.com
GitHub - quaris628/EinsteinEditor: An artful way to edit your bibite's brains!
Opens in a new window

github.com
warquys/BibitesModsLoader: A Library for easily implement mods for The Bibites Project. - GitHub
Opens in a new window

github.com
YBKy/Constance-Mod: A Mod for the artificial life game "The Bibites" - GitHub
Opens in a new window

reddit.com
[Mod] Introducing the Constance Mod : r/TheBibites - Reddit
Opens in a new window

steamdb.info
The Bibites 0.6.3: Corpses and Seasons · The Bibites: Digital Life update for 26 February 2026 - SteamDB
Opens in a new window

the-bibites.fandom.com
Modifying .bb8 files with Notepad | The Bibites Wiki - Fandom
Opens in a new window

thebibites.itch.io
The Bibites 0.5.0: Modernity and Progress
Opens in a new window

github.com
YBKy/Bibites-Quickfixes: Bugfixes for the artificial life game "The Bibites" - GitHub
Opens in a new window

github.com
YBKy/The-Bibites-Vanilla-Expanded-Modpack - GitHub
Opens in a new window

the-bibites.fandom.com
Modding with BepInEx (Tutorial) - The Bibites Wiki - Fandom
Opens in a new window

the-bibites.fandom.com
Modding with dnspy (Tutorial) - The Bibites Wiki - Fandom
Opens in a new window

reddit.com
simulation update: migration and irradiation : r/TheBibites - Reddit
Opens in a new window

reddit.com
Basic bibite evolves into predator in static simulation (0.6.0.1, large vanilla three islands) - 1600 generations timelapse video : r/TheBibites - Reddit
Opens in a new window

thebibites.itch.io
Comments 83 to 44 of 225 - The Bibites - itch.io
Opens in a new window

