# Local Windows broadcast fallback

This package runs one temporary broadcast world on a Windows NVIDIA GPU.
Use it while the AWS account does not have enough cloud GPU quota.

The world is a participant of the public map.
It runs its own sidecar, holds its own peer identity, and exchanges migrations with its neighbours.

All website viewers receive the same stream.
The spectator director follows one Bibite until it dies.
It then selects the youngest living Bibite.
The simulation runs at `6.5` times normal speed.

The camera uses a zoom of `250`, which is a wider view than the mod default.
It shows the selected Bibite's vision range.
It shows the brain panel for 15 seconds and the biology panel for 30 seconds.
It disables automatic spawns from the `Basic bibite` template.
Existing Bibites can still reproduce.
The website adds no separate simulation-speed label.

The world uses a Global Fertility of `3.5E-05` E/u²s, which is 3.5 times the game default.
More food lets more young Bibites live, so the population on camera stays dense.
The mod applies this value at each world load, and it changes the food supply only.

Read the full [live broadcast design](../../docs/live-broadcast.md) before installation.

## Publish on demand

The publisher sends video only while somebody watches.
It costs approximately 780 GB of inbound transfer each month while it runs, and that cost is the
same with an audience and with an empty room.

The service host cannot start or stop this publisher.
The publisher is OBS on this Windows machine, and the host holds no credential for it.
The host publishes the audience instead, and this package reads it:

```text
GET https://<service-domain>/api/viewers
{"watching":true,"hlsSessions":1,"lastViewerRequestAgeSec":4,"asOf":"2026-08-16T21:04:00Z"}
```

Read the [live broadcast design](../../docs/live-broadcast.md), *Publish on demand*, for the
endpoint contract. This package owns only the publisher half of it.

`watch-viewers.ps1` is that half. It runs beside the game, the sidecar, and OBS:

| Behaviour | Value |
|---|---|
| Poll interval | 10 seconds |
| Start | Immediately, when a reading says somebody is watching |
| Stop | After 180 unbroken seconds of nobody watching |
| Hysteresis | One reading of `watching` cancels the idle timer |
| Unreadable signal | Hold the current stream state, and write an alert line after 5 minutes |
| Log | `%LOCALAPPDATA%\BibitesMultiverse\broadcast\logs\watch-viewers.log`, in UTC |

An unreachable endpoint, an HTTP error, an unparsable document, and a presence document older than
120 seconds all read as unknown, and unknown holds the current state.
The other direction would take a live broadcast off the air because a status timer stopped.

The watcher's own poll is not an audience.
The service counts requests for `/watch` and `/stream/`, and the watcher reads neither, so it
cannot hold itself up.

While nobody watches, OBS keeps running and its scene keeps its game-capture hook.
Only the stream output stops, so the next start is one request and not an application launch.
The world keeps simulating and keeps its place on the map while the stream is quiet.

OBS still starts its stream when the broadcaster starts.
An operator can therefore verify a start before any audience exists, and the watcher takes the
stream down again about three minutes later if the room is still empty.

The watcher starts and stops the stream through obs-websocket on `127.0.0.1:4466`.
That is not the plugin's default port, because an unrelated OBS on the same desktop uses the
default and two servers cannot hold one port.
Use `BIBITES_OBS_WEBSOCKET_PORT` to change it.
OBS reads that configuration only when it starts, so enabling it is part of a stop and start.

Run the watcher's decisions without touching OBS:

```sh
./deploy/local-broadcast/test-watch-viewers.sh
```

### Stream profile

The encoder publishes `1280x720` at 30 frames each second and `1500` kbit/s of video with `64`
kbit/s of audio, through NVENC.
OBS rewrites its profile when it exits, so change `obs-basic.ini` and the installed `basic.ini`
while OBS is not running.

## Identity boundary

The installer creates private copies of the game and OBS below the Windows user profile.
It starts a new world in that private game copy.

The world enrolls its own map identity through the public enrollment endpoint.
It never copies an existing world's credential, so it cannot create a divergent copy of another
world.

A later installation reuses that identity. If the identity files are present but unreadable, the
installer stops before it changes the identity or runtime. A second enrollment can abandon this
world's place on the map.

The installer makes sure that the completed identity is valid before it stops the broadcaster.
It uses an exact enrollment retry to make sure that the credential is valid.
It also makes sure that the recorded world and durable sidecar peer ID match.
If these values disagree, the installer stops before it replaces a runtime file.
If custody data exists without a durable peer ID, the installer stops before enrollment.

One installation lock covers the preflight, file updates, and optional start.
A second installer stops while the first installer holds this lock.

The installer applies a protected Windows ACL to the full identity directory.
The ACL gives access to the current Windows user only.
If the installer cannot apply this ACL, it does not contact the enrollment endpoint.
It also stops if another account still has access after it applies the ACL.

The installer can find a pending record beside a completed identity.
It removes the pending record only when its peer ID and secret match the completed identity.
If they do not match, the installer keeps both records and stops.

Three files below `%LOCALAPPDATA%\BibitesMultiverse\broadcast\multiverse\` carry the identity:

| File | Contents |
|---|---|
| `peer-secret.txt` | The map credential secret |
| `enrollment.json` | The peer identity and relay address |
| `data\peer-id` | The durable peer identity that the sidecar uses |

The private OBS profile contains the RTMP publish password.
The installer reads the password from the origin through SSH.
It does not put the password in a process argument or this repository.

The private OBS also holds the obs-websocket password that the viewer watcher uses.
The installer keeps an existing password and generates one only when none exists.
The watcher reads it from that file, so no obs-websocket password reaches a command line either.

Do not print, copy, or commit the OBS `service.json` file, the obs-websocket `config.json` file,
`peer-secret.txt`, or `enrollment.json`.

## Requirements

Prepare this software before installation:

- A Windows user session with the supported Bibites game and BepInEx.
- OBS Studio and an NVIDIA GPU that supports NVENC.
- WSL with systemd, AWS CLI v2, Session Manager Plugin, `flock`, `tmux`, `jq`, Go, and .NET SDK.
- SSH access to the private stream origin.
- An AWS profile that can read the cloud stack and start an SSM session.
- Outbound HTTPS to the public map for enrollment, and outbound WSS for the relay.

The default source paths are the Steam game directory and the standard OBS directory.
Use `BIBITES_LOCAL_GAME_DIR` or `BIBITES_WINDOWS_OBS_DIR` for other paths.

## Install

Set the live topology from private operations storage:

```sh
export AWS_PROFILE=<profile>
export AWS_REGION=<region>
export BIBITES_AWS_ACCOUNT_ID=<12-digit-account-id>
export BIBITES_STREAM_ORIGIN_SSH=<user@origin-host>
export BIBITES_STREAM_ORIGIN_PRIVATE_IP=<private-origin-address>
```

Install and start the broadcaster:

```sh
./deploy/local-broadcast/install.sh
```

Set `BIBITES_LOCAL_WORLD_NAME` or use `--world <name>` to change the world name.
Use `--no-start` to install the files without starting the broadcaster.

These variables change the map settings. The defaults match a participant install:

| Variable | Default | Meaning |
|---|---|---|
| `BIBITES_PUBLIC_MAP` | `release/kit/public-map.json` | Public join configuration |
| `BIBITES_BROADCAST_SIDECAR_PORT` | `8787` | Contract A listener on loopback |
| `BIBITES_BROADCAST_EXPORT_EDGES` | `E,N,W,S` | Edges this world exports through |
| `BIBITES_BROADCAST_EXCLUDE_SPECIES` | `Basic bibite` | Species that never migrates |

The first installation copies the game and OBS, and enrolls the world's identity.
Later installations reuse those copies and that identity, and update the Multiverse plugin,
the sidecar, and the configuration.

The installer makes sure that the map identity is valid before it stops a running broadcaster.
The sidecar runner makes sure that the durable peer ID matches before each start.

## Operate

Start the tunnel, sidecar, game, and OBS:

```sh
~/.local/lib/bibites-local-broadcast/bin/start
```

Make sure that the WSL supervisors are active:

```sh
systemctl --user is-active bibites-local-broadcast-tunnel.service
tmux -L bibites-broadcast has-session -t bibites-local-broadcast
```

Read the viewer watcher:

```sh
powershell.exe -NoProfile -Command \
  'Get-Content "$env:LOCALAPPDATA\BibitesMultiverse\broadcast\logs\watch-viewers.log" -Tail 20' \
  | tr -d '\r'
```

The first line of each run reports the presence address, the poll interval, and the idle period.
A `WARN` line reports one unreadable reading, and an `ALERT` line reports five minutes of them.
The watcher holds the stream state through both, so an alert is a signal to read the service, not
evidence that the broadcast stopped.

Read the spectator status:

```sh
powershell.exe -NoProfile -Command \
  'Get-Content "$env:LOCALAPPDATA\BibitesMultiverse\broadcast\state\director.json"' \
  | tr -d '\r' | jq
```

The status must report a zoom of `250` and a target time scale of `6.5`.
The `panel` value alternates between `brain` and `biology`.
The `fieldOfView` value must be `true`.
The `disabledSpawnSettings` value must be `1`.
The `fertility` value must be `3.5E-05`, which `jq` can print as `0.000035`.

Make sure that the world is on the map:

```sh
peer="$(jq -r .peerId \
  "$(wslpath -u "$(powershell.exe -NoProfile -Command \
    '[Environment]::GetFolderPath("LocalApplicationData")' | tr -d '\r')")/BibitesMultiverse/broadcast/multiverse/enrollment.json")"
curl -fsS https://<service-domain>/api/status |
  jq -e --arg peer "$peer" '
    any(.slots[]?;
      .peerId == $peer and .live == true and .modConnected == true and
      ((.exportEdges // []) | sort) == (["E","N","W","S"] | sort))'
```

Read `%LOCALAPPDATA%\BibitesMultiverse\broadcast\logs\sidecar.log` when that check fails.
The line `contract B: slot granted` reports the world's place on the map.
The `modConnected` value shows that the game mod reached the sidecar.
The `exportEdges` value must contain the configured map edges.

The website names this world only when the hosted archive is told which peer it is.
Set `MV_BROADCAST_PEER_ID` to the value printed above, then run
`provision.sh --only envfiles` on the service host and restart the archive.
Read the [live broadcast design](../../docs/live-broadcast.md), *Naming the world on the pages*.
Until then both pages say the world is unknown.

Make sure that the public page and the complete HLS session are available.
These requests are themselves an audience, so the first one starts the publisher if it is quiet.
Allow approximately 20 seconds and repeat the HLS check until it passes.
Run these commands from the repository root:

```sh
curl -fsS https://<service-domain>/watch >/dev/null
(
  source ./deploy/local-broadcast/install.sh
  hls_stream_ready 'https://<service-domain>/stream/bibites/index.m3u8'
)
```

The HLS function keeps the cookies that MediaMTX returns during the master redirects.
It reads the advertised child playlist and its latest completed segment with that session.
The segment must return HTTP status `200` and contain at least one byte.
A child request without `hlsSession` can return `401` while the stream is healthy.
The function rejects an absolute reference or a parent-directory reference.
It removes its temporary cookie and playlist files.

Stop only the private broadcast processes:

```sh
~/.local/lib/bibites-local-broadcast/bin/stop
```

The stop script compares the recorded process paths.
It does not stop another Bibites, OBS, or sidecar process.
It stops the viewer watcher first, because the watcher is the one process that can start the stream
and must not publish into an OBS that is already closing.
It matches the watcher by its command line, because the watcher is an ordinary `powershell.exe`.
It stops the game before the sidecar, so the world saves while its sidecar still holds custody.

Prove that all four processes are gone before you start again.
A start over a running set opens a second copy of this world under one map identity:

```sh
powershell.exe -NoProfile -Command \
  'Get-Process -Name "The Bibites","obs64","multiverse-sidecar" -ErrorAction SilentlyContinue |
     Where-Object { $_.Path -like "$env:LOCALAPPDATA\BibitesMultiverse\broadcast\*" } |
     Select-Object Id,Name,Path' | tr -d '\r'
```

## Limits

Keep the computer powered on and keep the Windows user logged in.
WSL must also remain running.
A Windows, WSL, display, or GPU restart can interrupt the broadcast.

The installer does not enable automatic start at boot.
Run the start command after a computer restart.
The website shows its waiting state until the publisher returns.

A stopped publisher and a quiet publisher look the same from outside.
Read the watcher log before you treat an empty player as a fault.

A stopped broadcast world is an ordinary absent world. Its place on the map is kept, and its
neighbours route around it until it returns.

This fallback is not a durable cloud deployment.
Replace it with the AWS GPU host after the account has enough quota.
