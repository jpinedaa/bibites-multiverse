# Live broadcast

The public page at [bibitesmultiverse.com/watch](https://bibitesmultiverse.com/watch)
shows one shared game camera. All viewers watch the same encoded stream.

## Deployment state

| Layer | State on 2026-08-14 | Evidence |
|---|---|---|
| `/watch` page | Live | The public page and sitemap return `200` |
| HTTPS HLS origin | Live | A synthetic H.264/AAC stream returned an LL-HLS manifest through `/stream/` |
| Private RTMP ingest | Live | MediaMTX listens on `172.26.12.110:1935` only |
| GPU broadcast host | Blocked by AWS quota | Both 4-vCPU quota requests have open support cases |

The page shows its reconnecting state until the GPU publisher starts. The relay,
archive, and cloud worlds continue to run during a broadcast outage.

The Spot quota request ID is
`c171d26492474f62abd094d9bea3523cRsTQdAHf`. The On-Demand request ID is
`9c9e863820334800a6ff900f19aecddfvNCyjoF7`.

## Data path

```text
GPU world -> FFmpeg/NVENC -> private RTMP -> MediaMTX -> loopback HLS -> nginx -> all viewers
                                  172.31/16      Lightsail       /stream/
```

The GPU host publishes one path named `bibites`. MediaMTX rejects a second
publisher for that path. Thus, every viewer receives the same camera timeline.

The public website does not contain the publish address or publish password.
Viewers can send only `GET` and `HEAD` requests to `/stream/`.

## Camera rule

The `SpectatorDirector` component uses this rule:

1. Select the youngest living Bibite.
2. Use the lowest entity ID to break an exact age tie.
3. Keep the selected Bibite when a younger Bibite is born.
4. Select a new Bibite after the current Bibite dies or leaves the world.
5. Keep the camera zoom at `35` world units by default.

The director uses the game selection API. That API sends the selected object to
`CameraManager`, which follows the object in `LateUpdate`.

The director changes the selection, camera zoom, and optional UI visibility. It
does not move, feed, heal, kill, export, or edit a Bibite.

These environment variables control the director:

| Variable | Default | Meaning |
|---|---:|---|
| `MULTIVERSE_BROADCAST` | `false` | Start the spectator director |
| `MULTIVERSE_BROADCAST_ZOOM` | `35` | Set the orthographic camera size |
| `MULTIVERSE_BROADCAST_RESELECT_DELAY` | `2` | Wait before the next selection |
| `MULTIVERSE_BROADCAST_STATUS_FILE` | empty | Write the current subject as JSON |
| `MULTIVERSE_BROADCAST_HIDE_UI` | `false` | Hide the main game UI |

## Service boundary

MediaMTX 1.20.0 runs as `multiverse-stream.service` on the Lightsail host.
The install script compares the release SHA-256 before installation.

The RTMP listener uses the Lightsail private address. The host firewall accepts
port `1935` only from the EC2 VPC CIDR, `172.31.0.0/16`.

The publisher also needs a random 64-hex-character password. The origin stores
this password in `/etc/multiverse/stream-publish.env` with mode `0640`.

The HLS and metrics listeners use loopback. nginx publishes HLS below
`/stream/` with rate and connection limits.

MediaMTX keeps stream segments in memory. It does not record the broadcast.
The archive remains the permanent migration record.

## GPU host

The GPU stack uses `g4dn.xlarge` Spot capacity in `us-east-1a`. The instance has
one NVIDIA T4 GPU, 4 vCPUs, and 16 GiB of memory.

The stack uses the official AWS Deep Learning Base AMI for Ubuntu 24.04. This
AMI contains the NVIDIA driver. Xorg renders a virtual `1280x720` display.

FFmpeg captures that display at 30 frames per second. NVENC produces a
2.5-Mbit/s H.264 stream with a two-second keyframe interval.

The stack has no inbound security-group rules. Systems Manager provides the
operator shell. The host publishes through Lightsail VPC peering.

The data volume starts from an EBS snapshot of the source world. CloudFormation
retains this volume after stack deletion.

CAUTION: Stop and disable the source world before you make the snapshot. Two
copies with one credential can create different descendants from one checkpoint.

## GPU deployment

Complete these steps after AWS grants at least 4 G-instance vCPUs:

1. Build and stage the current runtime.

   ```sh
   ./cloud/aws/build-artifacts.sh
   BIBITES_DISABLED_WORLD_IDS=slot-1 ./cloud/aws/make-manifest.sh
   ./cloud/aws/stage-artifacts.sh
   ```

2. Stop and disable `slot-1` on the source host.

   ```sh
   sudo systemctl disable --now bibites-game@slot-1.service
   sudo systemctl disable --now bibites-sidecar@slot-1.service
   ```

3. Make an EBS snapshot of `vol-0d51c430a70521630`.
4. Wait until the snapshot state is `completed`.
5. Deploy the GPU host with that snapshot.

   ```sh
   BIBITES_BROADCAST_SNAPSHOT_ID=snap-xxxxxxxx \
     ./cloud/aws/deploy-broadcast.sh
   ```

6. Make sure that the source services remain disabled.
7. Make sure that the public manifest returns `200`.

   ```sh
   curl -fsS -L https://bibitesmultiverse.com/stream/bibites/index.m3u8
   ```

8. Open `/watch` from a second network.

If the GPU deployment fails, do not start both copies. Delete the failed GPU
stack first. Then start the source world again.

## Availability and cost

Spot interruption stops the publisher first. The watcher then stops the game,
waits for save-on-quit, stops the sidecar, and flushes the volume.

The page automatically returns to its reconnecting state. A later instance
restart uses the same retained volume and the same world identity.

The observed `g4dn.xlarge` Spot price was `$0.3018/hour` on 2026-08-14. A full
month at that price is approximately `$220`, before storage and public IPv4.

One continuously open viewer uses approximately 810 GB each month at 2.5 Mbit/s.
The 3 TB Lightsail allowance supports about three such viewer-months.

Add a video CDN before the audience exceeds this direct-origin limit. The page
and origin do not depend on one CDN provider.

## Checks

Run these checks after an origin change:

```sh
sudo systemctl is-active multiverse-stream nginx
sudo ss -ltnp | grep -E ':(1935|8888|9998) '
sudo /opt/multiverse/deploy/provision.sh --only verify
curl -fsS https://bibitesmultiverse.com/watch
```

The expected listeners are:

- `172.26.12.110:1935` for private RTMP ingest.
- `127.0.0.1:8888` for HLS.
- `127.0.0.1:9998` for MediaMTX metrics.
