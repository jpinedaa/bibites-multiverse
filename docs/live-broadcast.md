# Live broadcast design

The optional broadcast shows one shared game camera.
All viewers receive the same encoded stream.

This document describes the public architecture and camera behavior.
It does not describe a current deployment, quota request, resource identifier, address, or price.

## Data path

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
The host firewall must accept port `1935` only from the publisher network.

The publisher also needs a random 64-character hexadecimal password.
Store that password in a protected host file or secret manager.
Do not put it in Git, process arguments, or a public runbook.

The HLS and metrics listeners use loopback.
nginx applies public rate and connection limits.

MediaMTX keeps stream segments in memory.
It does not create a recording.
The archive remains the durable migration record.

## GPU host

`cloud/aws/broadcast-template.yaml` defines the optional AWS GPU host.
The selected instance must support NVIDIA NVENC.

The stack uses an AWS Deep Learning Base AMI with an NVIDIA driver.
Xorg creates a virtual `1280x720` display.

FFmpeg captures that display at 30 frames each second.
NVENC produces a 2.5-Mbit/s H.264 stream with a two-second keyframe interval.

The stack has no inbound security-group rules.
Systems Manager provides the operator shell.

The data volume starts from an approved EBS snapshot of one source world.
CloudFormation retains the volume after stack deletion.

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

## Deployment procedure

Complete these steps after the account has enough GPU Spot quota:

1. Build and stage the current runtime and private manifest.
2. Disable the selected world in the staged manifest.
3. Stop and disable that world on the source host.
4. Create an EBS snapshot of the source data volume.
5. Wait until the snapshot state is `completed`.
6. Set the required `BIBITES_*` deployment variables.
7. Run `cloud/aws/deploy-broadcast.sh`.
8. Make sure that the source world remains disabled.
9. Check the HLS manifest through the public front door.
10. Open the watch page from a second network.

The deployment wrapper checks that the source services are stopped and disabled.
It also checks that the staged manifest disables the selected world.

The wrapper requires an existing SSM publish-password parameter.
It does not read or copy the password value.

If deployment fails, do not start both copies.
Remove or stop the failed copy before you restart the source world.

## Availability

A Spot interruption stops the publisher first.
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
