# Live broadcast design

The proposed broadcast shows one shared game camera.
All viewers receive the same encoded stream.

The AWS publisher is currently disabled by a security gate.
No public procedure in this repository deploys a working publisher.

This document describes the public architecture and camera behavior.
It does not describe a current deployment, quota request, resource identifier, address, or price.

## Target data path

```text
game world -> Xorg -> FFmpeg/NVENC -> private RTMP -> MediaMTX -> loopback HLS -> nginx -> viewers
```

The publisher sends one named stream to a private RTMP listener.
MediaMTX rejects a second publisher for that stream path.

MediaMTX exposes HLS on loopback.
nginx publishes HLS below `/stream/` over HTTPS.

The website does not contain the publish address or password.
Viewers can send only `GET` and `HEAD` requests to the public stream path.

The relay, archive, and headless worlds do not depend on the broadcaster.
A broadcast failure does not stop the map.

## Camera rule

The `SpectatorDirector` component uses this rule:

1. Select the youngest living Bibite.
2. Use the lowest entity ID to resolve an exact age tie.
3. Keep the selected Bibite when a younger Bibite is born.
4. Select a new Bibite after the current Bibite dies or leaves.
5. Use a default camera zoom of `35` world units.

The director uses the game selection API.
`CameraManager` follows the selected object in `LateUpdate`.

The director changes selection, camera zoom, and optional UI visibility.
It does not move, feed, heal, kill, export, or edit a Bibite.

These environment variables control it:

| Variable | Default | Meaning |
|---|---:|---|
| `MULTIVERSE_BROADCAST` | `false` | Start the spectator director. |
| `MULTIVERSE_BROADCAST_ZOOM` | `35` | Set the orthographic camera size. |
| `MULTIVERSE_BROADCAST_RESELECT_DELAY` | `2` | Wait before the next selection. |
| `MULTIVERSE_BROADCAST_STATUS_FILE` | empty | Write the current subject as JSON. |
| `MULTIVERSE_BROADCAST_HIDE_UI` | `false` | Hide the main game UI. |

## Origin boundary

`install-stream-origin.sh` installs a pinned MediaMTX release and checks its SHA-256 value.

The RTMP listener must use a private RFC1918 address.
It must listen on port `1935`.
The host firewall must accept port `1935` only from the publisher network.

The publisher subnet needs an active private route to the relay and RTMP addresses.
The deployment wrapper checks the effective subnet route table for both addresses.
It rejects a public destination and a route that uses the default internet path.

The publisher also needs a random 64-character hexadecimal password.
Store the parameter as a `SecureString` with the default `aws/ssm` KMS key.
Do not put it in Git, process arguments, or a public runbook.

FFmpeg needs the authenticated RTMP URL as a process argument in the current design.
That interface exposes the password to process inspection.
For this reason, the public AWS publisher does not start FFmpeg.

The HLS and metrics listeners use loopback.
nginx applies public rate and connection limits.

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

Do not deploy the broadcaster from this repository.
`cloud/aws/deploy-broadcast.sh` performs read-only preflight checks and then stops.
It does not send the source-host command or call CloudFormation.

The preflight checks these properties:

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

The private operations record must track interruption events and restart evidence.

## Capacity

A 2.5-Mbit/s stream uses approximately 810 GB for one continuously open viewer-month.
Direct-origin viewing therefore has a small audience limit.

Add a video CDN or managed video service before transfer exceeds the approved budget.
The page and origin do not depend on one CDN provider.

Read current compute, storage, public-address, and transfer prices before deployment.
Store the quote and forecast in private operations storage.

## Checks

Run these checks after an origin change:

```sh
sudo systemctl is-active multiverse-stream nginx
sudo ss -ltnp | grep -E ':(1935|8888|9998) '
sudo /opt/multiverse/deploy/provision.sh --only verify
curl -fsS https://<service-domain>/watch
```

The expected listeners are:

- A private RFC1918 address on port `1935` for RTMP ingest.
- `127.0.0.1:8888` for HLS.
- `127.0.0.1:9998` for MediaMTX metrics.

Check the public HLS manifest from outside the origin network.
Do not expose the RTMP or metrics listener to the internet.
