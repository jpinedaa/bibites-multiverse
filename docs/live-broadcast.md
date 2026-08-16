# Live broadcast design

The broadcast shows one shared game camera.
All viewers receive the same encoded stream.

The world under that camera is a participant of the public map.
It runs its own sidecar, holds its own peer identity, and exchanges migrations with its neighbours.
A Bibite the camera follows can therefore leave the world, and the camera then chooses another.

The AWS GPU publisher is currently disabled by a security gate.
The local Windows fallback remains available.
No public procedure in this repository deploys the AWS publisher.

This document describes the public architecture, the camera behavior, and the world settings the
broadcast profile applies.
It does not describe a current deployment, quota request, resource identifier, address, or price.

## Target data path

```text
game world -> GPU capture -> hardware H.264 -> private RTMP -> MediaMTX -> loopback HLS -> nginx -> viewers
```

The publisher sends one named stream to a private RTMP listener.
MediaMTX rejects a second publisher for that stream path.

MediaMTX exposes HLS on loopback.
nginx publishes HLS below `/stream/` over HTTPS.

MediaMTX redirects the first master request to make sure that the client accepts cookies.
The redirected master creates an HLS session and returns its session cookie.
Child playlists and segments must use the same cookie engine.
A child request without `hlsSession` can return `401` while the stream is healthy.

The website does not contain the publish address or password.
Viewers can send only `GET` and `HEAD` requests to the public stream path.

The relay, archive, and headless worlds do not depend on the broadcaster.
A broadcast failure does not stop the map.
A broadcast world that stops is an ordinary absent world: its sidecar keeps its place, and its
neighbours route around it.

## Map identity

The broadcast world joins the map through the same public enrollment as a participant world.
Read [`contracts/public-enrollment.md`](../contracts/public-enrollment.md) for the endpoint.

Six rules bind every broadcast publisher:

1. A publisher that starts a new world enrolls its own identity. It never copies a live credential.
2. A publisher that continues an existing world takes that world's identity only after the source
   world is stopped and disabled. One identity runs in one place, because two copies of one
   identity can create divergent descendants.
3. An installer that cannot read its own completed identity stops. It does not enroll again,
   because a second enrollment abandons the world's current place on the map.
4. An installer makes sure that the completed identity is consistent before it stops a running
   publisher. This process includes the credential, recorded world, and durable sidecar peer ID.
5. A pending record beside a completed identity must name the same peer ID and secret. The installer
   stops when either value differs.
6. If custody data exists without a durable peer ID, the installer stops before enrollment.

The identity and its secret stay outside this repository, below the publisher's user profile.
The local installer gives the identity directory a protected Windows ACL.
Only the current Windows user receives access.

## Naming the world on the pages

The two public pages tell the reader which world is on camera.
Neither page discovers this: no frame on either wire says that a world is being filmed.

The deployment tells the archive with `--broadcast-peer`, or with
`MULTIVERSE_BROADCAST_PEER` in `/etc/multiverse/archive.env`.
`provision.sh` writes that value from `MV_BROADCAST_PEER_ID`.
The peer id is a public identifier, not a secret.

The archive then publishes two fields on `/api/status`:

| Field | Meaning |
|---|---|
| `broadcastPeerId` | The peer id the deployment named. Absent when it named none |
| `broadcast` on one slot | True for that peer's slot only |

The broadcast page reads those fields and shows the slot number, the peer id, the position, and
a symbol of the map rectangle with that world's cell lit.
It uses the same `(column,row)` the live map prints, so a reader can hold the two pages together.
It also links to the live map.

The live map badges the same world in three places: its cell on the map, its row in the worlds
table, and its settings card.
Every badge links back to the broadcast page.
The cell badge is a pill marked `ON CAMERA` above the slot number.

A map cell is too narrow for a long peer id.
The cell therefore shows only as much of the id as fits beside the state label, and it ends a cut
id with an ellipsis.
The cell tooltip, the worlds table, the settings card, and `/api/status` all keep the whole id.

Three states are unknown, and each says so instead of naming a world:

1. The deployment named no world.
2. The named world is not on the map at the moment.
3. The archive is not answering.

Naming the wrong world is worse than naming none.
An operator who moves the camera to another world must change this value with it.

## Camera rule

The `SpectatorDirector` component uses this rule:

1. Select the youngest living Bibite.
2. Use the lowest entity ID to resolve an exact age tie.
3. Keep the selected Bibite when a younger Bibite is born.
4. Select a new Bibite after the current Bibite dies or leaves.
5. Use a default camera zoom of `35` world units.

The director uses the game selection API.
`CameraManager` follows the selected object in `LateUpdate`.

The director can also show the selected Bibite's panels and vision range.
It can request a fixed simulation speed for a broadcast.
Panel changes use real time, so the simulation speed does not affect their interval.

The director changes selection, presentation, and the requested simulation speed.
An optional rule can also change template spawn targets.
It does not move, feed, heal, kill, export, or edit a Bibite.

These environment variables control it:

| Variable | Default | Meaning |
|---|---:|---|
| `MULTIVERSE_BROADCAST` | `false` | Start the spectator director. |
| `MULTIVERSE_BROADCAST_ZOOM` | `35` | Set the orthographic camera size. |
| `MULTIVERSE_BROADCAST_RESELECT_DELAY` | `2` | Wait before the next selection. |
| `MULTIVERSE_BROADCAST_STATUS_FILE` | empty | Write the current subject as JSON. |
| `MULTIVERSE_BROADCAST_HIDE_UI` | `false` | Hide the main game UI. |
| `MULTIVERSE_BROADCAST_TIME_SCALE` | empty | Request a fixed simulation speed. |
| `MULTIVERSE_BROADCAST_PANELS` | empty | Rotate through `brain`, `biology`, or `expanded-brain`. |
| `MULTIVERSE_BROADCAST_PANEL_SECONDS` | `15` | Set the real-time interval between panels. |
| `MULTIVERSE_BROADCAST_SHOW_FOV` | `true` | Show the selected Bibite's vision range. |
| `MULTIVERSE_BROADCAST_DISABLE_SPAWN_TEMPLATES` | empty | Set named template spawn targets to zero. |

The panel rotation stays off when `MULTIVERSE_BROADCAST_HIDE_UI` is `true`.
Repeat a panel name to give that panel more time in each cycle.

The spawn option does not remove existing Bibites or stop natural reproduction.
A later world save records the zero target count.

The standard broadcast profile uses a zoom of `250` and a speed of `6.5`.
It shows the brain panel for 15 seconds and the biology panel for 30 seconds.
It also shows the selected Bibite's vision range.
It disables automatic spawns from the `Basic bibite` template.
It also sets the world's Global Fertility, which the next section describes.

## World fertility

A broadcast world can run with more food than the game default.
This is a world setting, not a camera setting, and the spectator director does not set it.
More food lets more young Bibites live, so the population on camera stays dense.
It changes the food supply only.
It does not add, move, feed, heal, kill, or edit a Bibite.

This environment variable controls it:

| Variable | Default | Meaning |
|---|---:|---|
| `MULTIVERSE_FERTILITY` | empty | Set the world's Global Fertility at each world load. |

The value is the game's `Global Fertility` in E/u²s, for example `3.5E-05`.
The game default is `1E-05`, which its own scale calls `Normal`.
The game accepts a positive value up to `5E-04`.
The mod refuses any other value with a warning and keeps the world's saved value.
An empty or unset variable writes nothing, which is what every other installation gets.

The mod writes the setting after the world loads.
Each zone then recomputes its pellet production, so the change is immediate.
The mod does not restore the previous value when the world unloads.
A periodic world save records the applied value while the world runs.
While the variable is set, the saved fertility is overwritten at each start.
Remove the variable and the world keeps the last value it saved.

The standard broadcast profile uses `3.5E-05`, the value of the shipped `Thick Soup` scenario.
The spectator status file reports the running value as `fertility`.
The broadcast world is a participant of the map, so it is richer than its neighbours.
Only Bibites migrate between worlds, so this setting does not change a neighbour's food.

## Origin boundary

`install-stream-origin.sh` installs a pinned MediaMTX release.
It compares the release SHA-256 value with the pinned value.

The RTMP listener must use a private RFC1918 address.
It must listen on port `1935`.
The host firewall must accept port `1935` only from the publisher network.

The publisher subnet needs an active private route to the relay and RTMP addresses.
The deployment wrapper evaluates the effective subnet route table for both addresses.
It rejects a public destination and a route that uses the default internet path.

The publisher also needs a random 64-character hexadecimal password.
Store the parameter as a `SecureString` with the default `aws/ssm` KMS key.
Do not put it in Git, process arguments, or a public runbook.

FFmpeg needs the authenticated RTMP URL as a process argument in the current design.
That interface exposes the password to process inspection.
For this reason, the public AWS publisher does not start FFmpeg.

The HLS and metrics listeners use loopback.
The HLS route uses a per-address connection limit.
It has no request-rate limit because low-latency HLS sends many small part requests.

MediaMTX keeps stream segments in memory.
It does not create a recording.
The archive remains the durable migration record.

## GPU host

`cloud/aws/broadcast-template.yaml` retains the proposed AWS GPU host interface.
The selected instance must support NVIDIA NVENC.
It must also support the `x86_64` architecture.

The stack uses an AWS Deep Learning Base AMI with an NVIDIA driver.
Xorg creates a virtual `1280x720` display.

In the target design, FFmpeg captures that display at 30 frames each second.
NVENC produces a 2.5-Mbit/s H.264 stream with a two-second keyframe interval.

The stack has no inbound security-group rules.
Systems Manager provides the operator shell.

The interface accepts only `slot-1` because its systemd dependencies use that slot.
The data volume starts from an approved EBS snapshot of that source world.
CloudFormation retains the volume after stack deletion.

The host attaches the tagged volume during bootstrap.
CloudFormation waits for a host installation signal before it reports success.
The current installer exits with failure because the password transport is unresolved.
The signal trap reports that failure to CloudFormation.

CAUTION: Stop and disable the source world before you create the snapshot.
Two copies with one credential can create different descendants from one checkpoint.

## Local Windows fallback

Use [`deploy/local-broadcast/`](../deploy/local-broadcast/README.md) while cloud GPU quota is not available.
This fallback runs one new map world on a Windows NVIDIA GPU.
It does not copy an existing map world or an existing map credential.

The installer creates private copies of the game and OBS below the Windows user profile.
It builds a Windows sidecar, enrolls one new identity with the public map, and keeps that
identity for every later installation.
One exclusive lock covers identity preflight, runtime updates, and startup.
The preflight finishes before the installer stops a live process or replaces a runtime file.
OBS captures the game process and uses NVENC to publish H.264.
A WSL service opens a private RTMP tunnel through AWS Systems Manager.
A dedicated `tmux` session supervises the Windows sidecar, game, and OBS processes.

The sidecar starts first, because it mints the Contract A token the mod presents and takes the
world's place on the map before the game opens.
The runner refuses to start when its configured peer ID differs from the durable peer ID.
The stop order is the reverse: OBS, then the game, then the sidecar.

The fallback uses the standard broadcast profile.
It exports all four edges and keeps `Basic bibite` out of migration, which is the participant
default.
The website adds no separate simulation-speed label.

The computer must stay powered on, and the Windows user must stay logged in.
Windows, WSL, display, or GPU restarts can interrupt the stream.
Run the fallback start command again after a computer restart.

## Deployment inputs

Keep these inputs in private operations storage:

- The approved AWS account and region.
- The source stack and world identifier.
- The completed snapshot identifier.
- The VPC, subnet, and availability zone.
- The private relay and RTMP addresses.
- The relay certificate name.
- The publish-password parameter name.
- The current instance type, quota, and cost forecast.

Do not add these live values to this document.

## Deployment gate

Do not deploy the AWS broadcaster from this repository.
`cloud/aws/deploy-broadcast.sh` performs read-only preflight checks and then stops.
It does not send the source-host command or call CloudFormation.

The preflight requires these properties:

- The staged manifest disables `slot-1`.
- The snapshot is complete, encrypted, and owned by the approved account.
- The source stack resolves to one online Linux instance.
- The instance type supports `x86_64` and reports an NVIDIA GPU.
- The Spot quota covers the type's `DefaultVCpus` value.
- The publish parameter is a default-key `SecureString`.
- The relay and RTMP addresses use port and private-route rules.

The route check selects the longest matching route.
It rejects the destination if that effective route is blackholed or not approved.
Approved targets are local, peering, transit gateway, and private virtual gateway routes.
The check fails closed if the selected route table contains a managed prefix-list route.
The current preflight does not resolve the networks inside managed prefix lists.

The wrapper reads parameter metadata only.
It does not decrypt or copy the publish password.

`cloud/aws/source-world-stopped.sh` contains the final source-host proof.
Its fixtures make each of the four state checks fail independently.
The game and sidecar must both be inactive and disabled.

Enable deployment only after a publisher can read credentials without putting them in arguments.
The credential must also stay out of status output and logs.
Do not add an override that bypasses this gate.

## Planned availability

In the target design, a Spot interruption stops the publisher first.
The watcher then stops the game and waits for save-on-quit.
It stops the sidecar and flushes the volume.

The page returns to its reconnecting state.
A later instance uses the retained data and world identity.

The page ignores a rate-limited health probe after the player loads.
This prevents a false reconnecting message while the video continues.

The private operations record must track interruption events and restart evidence.

## Capacity

Transfer sets the audience limit, and the viewer cost is not the media rate.
One continuously open viewer of the 2.5-Mbit/s stream costs approximately 1,150 GB each month with
low-latency HLS, of which 32 percent is playlist re-fetching.
The same media without low latency costs approximately 790 GB each month and adds several seconds
of delay.
The publisher's own ingest costs approximately 780 GB each month while it runs, with no viewers.
Direct-origin viewing therefore has a small audience limit.

Read [`deploy/SIZING.md`](../deploy/SIZING.md), "Network transfer", for the units, the
measurements, and the rest of the model.
Change a figure there first.

Add a video CDN or a managed video service before direct-origin transfer exceeds the approved
budget.
Select the non-low-latency variant first, because a cache cannot help low-latency HLS: each
playlist request stays open until the next part exists.
The page and origin do not depend on one CDN provider.

Read current compute, storage, public-address, and transfer prices before deployment.
Store the quote and forecast in private operations storage.

## Checks

From the repository root, run these checks after an origin change:

```sh
sudo systemctl is-active multiverse-stream nginx
sudo ss -ltnp | grep -E ':(1935|8888|9998) '
sudo /opt/multiverse/deploy/provision.sh --only verify
curl -fsS https://<service-domain>/watch
(
  source ./deploy/local-broadcast/install.sh
  hls_stream_ready 'https://<service-domain>/stream/bibites/index.m3u8'
)
```

The HLS function keeps the server session from the master through the latest completed segment.
The segment must return HTTP status `200` and contain at least one byte.
It rejects an unsafe relative reference and removes its temporary files.

The expected listeners are:

- A private RFC1918 address on port `1935` for RTMP ingest.
- `127.0.0.1:8888` for HLS.
- `127.0.0.1:9998` for MediaMTX metrics.

Make sure that the public HLS manifest is available from outside the origin network.
Do not expose the RTMP or metrics listener to the internet.

Run these checks after a publisher change, with the broadcast world's peer identity:

```sh
curl -fsS https://<service-domain>/api/status |
  jq -e --arg peer '<broadcast-peer-id>' '
    any(.slots[]?;
      .peerId == $peer and .live == true and .modConnected == true and
      ((.exportEdges // []) | sort) == (["E","N","W","S"] | sort))'
curl -fsS https://<service-domain>/api/status |
  jq -e --arg peer '<broadcast-peer-id>' '.broadcastPeerId == $peer'
```

The broadcast world must be live with its game mod and all configured export edges.
A stream that runs while the map does not show that world is a publisher that lost its sidecar.

The second check is the pages' own claim.
A `broadcastPeerId` that names a different world makes both pages point at the wrong one.
