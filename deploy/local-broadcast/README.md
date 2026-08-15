# Local Windows broadcast fallback

This package runs one temporary broadcast world on a Windows NVIDIA GPU.
Use it while the AWS account does not have enough cloud GPU quota.

All website viewers receive the same stream.
The spectator director follows one Bibite until it dies.
It then selects the youngest living Bibite.
The simulation runs at `7.5` times normal speed.

The camera uses a zoom of `45`.
It shows the selected Bibite's vision range.
It alternates the brain and biology panels every 15 seconds.
The website adds no separate simulation-speed label.

Read the full [live broadcast design](../../docs/live-broadcast.md) before installation.

## Safety boundary

The installer creates private copies of the game and OBS below the Windows user profile.
It starts a new offline exhibition world in that private game copy.
It does not start a sidecar or load a Multiverse credential.
It cannot join the map or create migration edges.

The private OBS profile contains the RTMP publish password.
The installer reads the password from the origin through SSH.
It does not put the password in a process argument or this repository.
Do not print, copy, or commit the OBS `service.json` file.

## Requirements

Prepare this software before installation:

- A Windows user session with the supported Bibites game and BepInEx.
- OBS Studio and an NVIDIA GPU that supports NVENC.
- WSL with systemd, AWS CLI v2, Session Manager Plugin, `tmux`, and .NET SDK.
- SSH access to the private stream origin.
- An AWS profile that can read the cloud stack and start an SSM session.

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

Set `BIBITES_LOCAL_WORLD_NAME` or use `--world <name>` to change the exhibition world name.
Use `--no-start` to install the files without starting the broadcaster.

The first installation copies the game and OBS.
Later installations reuse those copies and update the Multiverse plugin and configuration.

## Operate

Start the tunnel, game, and OBS:

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

The status must report a zoom of `45` and a target time scale of `7.5`.
The `panel` value alternates between `brain` and `biology`.
The `fieldOfView` value must be `true`.

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
It does not stop another Bibites or OBS process.

## Limits

Keep the computer powered on and keep the Windows user logged in.
WSL must also remain running.
A Windows, WSL, display, or GPU restart can interrupt the broadcast.

The installer does not enable automatic start at boot.
Run the start command after a computer restart.
The website shows its reconnecting state until the publisher returns.

This fallback is not a durable cloud deployment.
Replace it with the AWS GPU host after the account has enough quota.
