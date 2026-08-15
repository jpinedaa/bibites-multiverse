# Local Windows broadcast fallback

This package runs one temporary broadcast world on a Windows NVIDIA GPU.
Use it while the AWS account does not have enough cloud GPU quota.

The world is a participant of the public map.
It runs its own sidecar, holds its own peer identity, and exchanges migrations with its neighbours.

All website viewers receive the same stream.
The spectator director follows one Bibite until it dies.
It then selects the youngest living Bibite.
The simulation runs at `7.5` times normal speed.

The camera uses a zoom of `75`, which is a wider view than the mod default.
It shows the selected Bibite's vision range.
It shows the brain panel for 15 seconds and the biology panel for 30 seconds.
It disables automatic spawns from the `Basic bibite` template.
Existing Bibites can still reproduce.
The website adds no separate simulation-speed label.

Read the full [live broadcast design](../../docs/live-broadcast.md) before installation.

## Identity boundary

The installer creates private copies of the game and OBS below the Windows user profile.
It starts a new world in that private game copy.

The world enrolls its own map identity through the public enrollment endpoint.
It never copies an existing world's credential, so it cannot create a divergent copy of another
world.

A later installation reuses that identity. If the identity files are present but unreadable, the
installer stops and changes nothing: a second enrollment would abandon this world's place on the
map.

The installer makes sure that the completed identity is valid before it stops the broadcaster.
It uses an exact enrollment retry to make sure that the credential is valid.
It also makes sure that the recorded world and durable sidecar peer ID match.
If these values disagree, the installer stops before it replaces a runtime file.

One installation lock covers the preflight, file updates, and optional start.
A second installer stops while the first installer holds this lock.

The installer applies a protected Windows ACL to the full identity directory.
The ACL gives access to the current Windows user only.
If the installer cannot apply this ACL, it does not contact the enrollment endpoint.
It also stops if the ACL contains access for another account.

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
Do not print, copy, or commit the OBS `service.json` file, `peer-secret.txt`, or `enrollment.json`.

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

Read the spectator status:

```sh
powershell.exe -NoProfile -Command \
  'Get-Content "$env:LOCALAPPDATA\BibitesMultiverse\broadcast\state\director.json"' \
  | tr -d '\r' | jq
```

The status must report a zoom of `75` and a target time scale of `7.5`.
The `panel` value alternates between `brain` and `biology`.
The `fieldOfView` value must be `true`.
The `disabledSpawnSettings` value must be `1`.

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

Make sure that the public page and HLS manifest are available:

```sh
curl -fsS https://<service-domain>/watch >/dev/null
curl -fsS -H 'Cookie: cookieCheck=1' \
  'https://<service-domain>/stream/bibites/index.m3u8?cookieCheck=1' >/dev/null
```

Stop only the private broadcast processes:

```sh
~/.local/lib/bibites-local-broadcast/bin/stop
```

The stop script compares the recorded process paths.
It does not stop another Bibites, OBS, or sidecar process.
It stops the game before the sidecar, so the world saves while its sidecar still holds custody.

## Limits

Keep the computer powered on and keep the Windows user logged in.
WSL must also remain running.
A Windows, WSL, display, or GPU restart can interrupt the broadcast.

The installer does not enable automatic start at boot.
Run the start command after a computer restart.
The website shows its reconnecting state until the publisher returns.

A stopped broadcast world is an ordinary absent world. Its place on the map is kept, and its
neighbours route around it until it returns.

This fallback is not a durable cloud deployment.
Replace it with the AWS GPU host after the account has enough quota.
