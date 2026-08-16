# AWS cloud-world kit

This directory contains reusable infrastructure for headless Bibites worlds on AWS.
It does not contain a live inventory, current quote, source-machine migration, or deployment receipt.

The main stack uses one persistent EC2 Spot host and one retained EBS data volume.
Each world has a separate game directory, configuration root, sidecar journal, and credential file.

The broadcast files preserve a proposed GPU-host interface.
The current deployment wrapper fails at a password-transport safety gate.
See [`docs/live-broadcast.md`](../../docs/live-broadcast.md) for the public design.

## Public and private inputs

The public repository contains these reusable parts:

- CloudFormation templates.
- Host installation and systemd files.
- Artifact build and upload scripts.
- Stack deployment, update, backup, and check scripts.

Keep these inputs outside the public repository:

- AWS account, subnet, VPC, address, volume, snapshot, and instance identifiers.
- Current Spot quotes, budgets, quotas, and cost reports.
- World saves and proprietary game archives.
- World-to-slot manifests for a live map.
- Join strings and credential values.
- Deployment receipts and recovery evidence.

The ignored `cloud/aws/dist/`, `cloud/aws/imports/`, and `cloud/aws/private/` paths are not archival storage.
Do not commit their contents.

## Required tools

The workstation needs these tools:

- AWS CLI v2.
- `file`, `jq`, `unzip`, `tar`, and `sha256sum`.
- Go and the .NET SDK for a runtime build.
- Permission to deploy CloudFormation, IAM, EC2, EBS, S3, SSM, and DLM resources.

Use a limited AWS identity with temporary credentials.
Do not use an AWS root access key.

## Account guard

Each mutating wrapper requires `BIBITES_AWS_ACCOUNT_ID`.
Set it to the approved account identifier from private operations storage.

```sh
export AWS_PROFILE=<profile>
export AWS_REGION=<region>
export BIBITES_AWS_ACCOUNT_ID=<12-digit-account-id>

aws --profile "$AWS_PROFILE" sts get-caller-identity
```

The scripts stop when the authenticated account differs from the approved value.

## Runtime architecture

The host uses an encrypted retained volume at `/srv/bibites`.

```text
/srv/bibites/
├── base/game/                 verified Linux game and BepInEx base
├── bin/                       sidecar binary
├── worlds/<world-id>/
│   ├── game/                  isolated game folder
│   ├── config/                Unity saves and preferences
│   ├── sidecar/               journal and identity state
│   ├── logs/
│   ├── peer-secret.txt        mode 0600, loaded from Parameter Store
│   └── world.env              mode 0600
└── metrics/performance.jsonl  local performance samples
```

The game folders use hard links for immutable base files.
Each world has a separate BepInEx log path.

Two watchers leave a breadcrumb on the runtime file system:

```text
/run/bibites-ops/
├── spot-watch.state   last IMDS poll: time, HTTP status, token age, consecutive failures
├── recycle.state      last recycle decision: time, available memory share, action, detail
└── recycle.last       start time of the last recycle, which the cool-off reads
```

Each state file is rewritten on every tick.
Its timestamp reports whether the watcher is alive, and its contents report the last decision.
`/run` is a memory file system, so these files are absent from boot until the first tick.
The broadcast host writes `broadcast-spot-watch.state` in the same directory.

The directory is deliberately not `/run/bibites`.
`bibites-game@%i.service` declares that path as its `RuntimeDirectory`, so systemd deletes and
creates it again on every world restart, including a restart the recycler performs.
A record must not live inside a directory that the thing it observes owns.

The security group has no inbound rules.
Use AWS Systems Manager Session Manager for administration.

## Manifest

Create a private JSON manifest with schema `1`.
The manifest identifies each world, save object, credential parameter, and placement.

Example:

```json
{
  "schema": 1,
  "worlds": [
    {
      "id": "world-a",
      "peerId": "peer-a",
      "worldName": "Example-World",
      "sidecarPort": 8787,
      "saveKey": "imports/Example-World.zip",
      "credentialParameter": "/bibites-multiverse/cloud/world-a/peer-secret",
      "position": "0,0",
      "preferredSlot": 1,
      "targetTimeScale": 1,
      "saveMinutes": 10,
      "saveKeep": 6,
      "enabled": true
    }
  ]
}
```

Keep the manifest and its saves in protected local or object storage.
The public repository does not generate a live manifest.

Put each secret half in the Parameter Store path named by `credentialParameter`.
Use the `SecureString` type and the default `aws/ssm` KMS key.
The deployment wrapper rejects plaintext parameters and customer-managed keys.
Do not place a complete join string in S3, a manifest, or a command argument.

The schema validator accepts from 1 through 100 worlds.
It applies these rules:

- `id` and `peerId` use the public 64-character identifier alphabet.
- `worldName` is a safe filename. It cannot contain a path separator or traversal segment.
- `sidecarPort` is a unique integer from `1024` through `65535`.
- `saveKey` is a safe S3 key that ends in `.zip`.
- `credentialParameter` is a safe absolute Parameter Store path.
- `position` uses `column,row`, with non-negative integer coordinates.
- `preferredSlot` is a positive integer when present.
- `targetTimeScale`, `saveMinutes`, and `saveKeep` use bounded numeric values.
- `enabled` is a Boolean value when present.

The validator rejects unknown fields and duplicate identity or placement values.
Run its fixture checks after a schema change:

```sh
for test in ./cloud/aws/test-*.sh; do "$test"; done
```

## Build artifacts

Set paths to authorized local archives:

```sh
export BIBITES_GAME_ZIP=/protected/path/TheBibites-Linux.zip
export BIBITES_BEPINEX_ZIP=/protected/path/BepInEx-linux-x64.zip
./cloud/aws/build-artifacts.sh
```

The build script compiles the sidecar for `linux/amd64`.
It checks that the result is an x86-64 ELF file.
It also checks the supported game assembly and creates a content-addressed runtime archive.

## Stage artifacts

Set the private manifest and save directory:

```sh
export BIBITES_MANIFEST_FILE=/protected/deployment/worlds.json
export BIBITES_SAVE_DIR=/protected/deployment/saves
./cloud/aws/stage-artifacts.sh
```

The stage script checks every save before it calls AWS.
Each save must be a valid ZIP file that contains `scene.bb8scene`.

The local save directory uses flat filenames.
The script rejects two save keys that have the same basename.

The script creates a private encrypted S3 bucket when necessary.
It blocks public access and uploads every artifact and save first.
It uploads `worlds.json` last, so a host cannot read incomplete references.

Record the bucket and object prefix in private operations storage.

## Apply a manifest update

Apply a private manifest change with this sequence:

```sh
./cloud/aws/stage-artifacts.sh
./cloud/aws/sync-host.sh
./cloud/aws/verify-host.sh
```

`stage-artifacts.sh` validates the manifest and every referenced save before an AWS mutation.
It uploads immutable runtime inputs and saves before it replaces `worlds.json`.

`sync-host.sh` asks the existing host to download that object.
The host applies the same schema validator before it changes world services.

The reconciliation has these semantics:

- A new enabled row creates its world directories, credential, initial save, and services.
- A disabled row stops and disables its services. Its data remains on the retained volume.
- A re-enabled row starts its existing world data again.
- An existing save remains unchanged during the normal synchronization.
- Updated environment or credential files affect a process after its next service start.
- A removed row does not stop an existing world. Disable the row before a later removal.

Keep a world disabled until its replacement or migration passes all checks.
Never run two copies with the same save and credential.

## Deploy the headless host

Set every topology input from private inventory:

```sh
export BIBITES_SUBNET_ID=<subnet-id>
export BIBITES_VPC_ID=<vpc-id>
export BIBITES_AVAILABILITY_ZONE=<availability-zone>
export BIBITES_RELAY_PRIVATE_IP=<private-relay-ip>
export BIBITES_RELAY_DOMAIN=<certificate-name>
export BIBITES_CREDENTIAL_PARAMETER_PREFIX=<parameter-path-prefix>
export BIBITES_INSTANCE_TYPE=<instance-type>
export BIBITES_DATA_VOLUME_GIB=<size>
```

If the relay uses Lightsail VPC peering, also set:

```sh
export BIBITES_ENABLE_LIGHTSAIL_PEERING=1
```

Deploy and check the host:

```sh
./cloud/aws/deploy-host.sh
./cloud/aws/deploy-backups.sh
./cloud/aws/verify-host.sh
```

The runtime and default AMI require `x86_64`.
The deployment wrapper queries EC2 and rejects an incompatible instance type.
Use the wrapper instead of a direct CloudFormation deployment.

The wrapper also checks every manifest credential parameter.
Each parameter must be a `SecureString` that uses the default `aws/ssm` KMS key.

The instance attaches its tagged data volume during bootstrap.
Its role can attach only the tagged project instance and volume.
CloudFormation waits for the host installation signal before it reports success.

The host template retains the data volume after stack deletion.
This protection also leaves a storage charge.
Track retained volumes in private inventory.

## Runtime behavior

Each sidecar starts before its game.
The game starts only after the relay grants the sidecar a slot.

The game uses `-batchmode -nographics`.
A shared Xvfb service provides the X11 backend required by the Linux build.

Each world saves on its configured schedule and retains the configured number of copies.
The Spot watcher receives the EC2 interruption notice.
It stops games in parallel, waits for save-on-quit, stops sidecars, and flushes the file system.

The watcher reads the HTTP status of each poll instead of discarding it.
IMDS answers `404` while no interruption is scheduled, which is the usual case, and the watcher
stays silent for it.
IMDS answers `401` or `403` when the IMDSv2 token has expired.
The watcher logs that status, mints a new token at once, and also refreshes the token at half its
lifetime.
It logs a heartbeat every ten minutes and records each poll in
`/run/bibites-ops/spot-watch.state`.

An earlier version minted one six-hour token, refreshed it only when the variable was empty, and
discarded the status.
It went blind after six hours of uptime with no log line and no failed unit, because a failed
credential and a quiet endpoint produced the same swallowed error.

The sidecar journal is durable before an acknowledgment.
Shutdown does not create that durability.

CAUTION: Do not run two copies from one save and credential.
The copies can create different descendants from one checkpoint.

CAUTION: Do not import a sidecar journal from another map.
Old custody records can target a different topology.

## Memory recycling

A world process grows by approximately `0.72 GiB` for each wall-clock hour.
The growth is independent of the configured time scale, the achieved rate, and the population.
Six worlds fill a 32 GiB host in about seven hours, and that host then reaches swap thrash and an
out-of-memory kill.
[`deploy/SIZING.md`](../../deploy/SIZING.md) carries the formula and the exhaustion model.

`bibites-recycle.timer` asks every ten minutes whether one world must restart.
`bibites-recycle-world` answers from live host memory.
It does nothing while available memory stays above its threshold.
Below that threshold it restarts one world, the largest that has been running for at least an hour.
It holds off while any world is dark, waits out a cool-off after each recycle, and refuses outright
when the chosen world does not enable `MULTIVERSE_SAVE_ON_QUIT` in its `world.env`.

The game saves on quit, so a recycle costs no simulated time.
`bibites-game@%i.service` wants `bibites-timescale@%i.service`, so a restarted world applies its
target time scale again without an operator command.
This matters for every restart the host performs by itself: the time-scale unit is a completed
`Type=oneshot`, and ordering alone does not run it again.

The host installer does not install this script and does not enable this timer.
Install and enable both explicitly on a host that runs worlds:

```sh
sudo install -m 0755 /opt/bibites-runtime/bibites-recycle-world \
  /usr/local/libexec/bibites-recycle-world
sudo /usr/local/libexec/bibites-recycle-world --dry-run
sudo systemctl enable --now bibites-recycle.timer
```

The dry run names the world it would choose and changes nothing.
Read [`deploy/RESTART-POLICY.md`](../../deploy/RESTART-POLICY.md) for the restart class, its
participant effect, and the command that stops it.

## Administration

Start a Session Manager shell:

```sh
instance_id=$(aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  cloudformation describe-stacks --stack-name "${BIBITES_STACK_NAME:-bibites-cloud-worlds}" \
  --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' --output text)

aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  ssm start-session --target "$instance_id"
```

On the host, show world state:

```sh
sudo bibites-cloud-status
sudo bibites-cloud-status --json | jq
```

Read a world log with its manifest identifier:

```sh
sudo tail -n 100 /srv/bibites/worlds/<world-id>/logs/sidecar.log
sudo tail -n 100 /srv/bibites/worlds/<world-id>/game/BepInEx/LogOutput.log
```

Read the performance record:

```sh
sudo tail -n 12 /srv/bibites/metrics/performance.jsonl | jq
sudo tail -n 1 /srv/bibites/metrics/performance.jsonl \
  | jq '{at, memoryAvailableBytes, pressureFullUsec, vm, gameMemoryBytes}'
```

Each line holds the host reading, the pressure counters, and every world's own view.
`pressureFullUsec` holds the pressure-stall totals in microseconds since boot for `cpu`, `memory`,
and `io`.
Each one counts the time in which no task on the host could run for want of that resource.
Divide by 1,000,000 to read seconds.
Read these before the load average, because utilization cannot separate a busy host from a stalled
host.
The first sample after this host was extended read zero for `cpu`, 3,707 seconds for `memory`, and
5,720 seconds for `io`.
That host had waited hours on memory and storage and had never waited on CPU.

`vm.pswpin` and `vm.pswpout` report swap traffic, and `vm.oomKill` turns "a world restarted" into
"the kernel killed it".
`gameMemoryBytes` holds resident bytes for each world, which names the world to act on.
A host total names none.

The pressure and `vm` counters are cumulative, so compare two samples instead of reading one.
A value the sampler cannot read is `null` and never `0`.

Check that each watcher is alive:

```sh
sudo cat /run/bibites-ops/spot-watch.state
sudo cat /run/bibites-ops/recycle.state
sudo systemctl list-timers bibites-recycle.timer
sudo journalctl -u bibites-spot-watch -n 20 --no-pager
sudo journalctl -u bibites-recycle.service -n 20 --no-pager
```

The Spot watcher polls every few seconds, so a `spot-watch.state` older than about a minute means
that it is not watching.
The recycler writes on each ten-minute tick, so `recycle.state` is stale after about eleven minutes.
A missing file after a long uptime means the same thing as a stale one.
The unit journal carries the reason.

## Runtime updates

Use an in-place runtime update for scripts, the plugin, or the sidecar:

```sh
./cloud/aws/build-artifacts.sh
./cloud/aws/stage-artifacts.sh
./cloud/aws/update-runtime.sh
./cloud/aws/verify-host.sh
```

CAUTION: Do not deploy the host stack only to update runtime files.
A launch-template change can replace the instance.

The update wrapper validates every stack value before it stops a world.
It also confirms that the data volume is attached and the Systems Manager target is online.

The host downloads and checks the new runtime before it stops a service.
A non-blocking host lock rejects a second activation.
Systems Manager gets one command and an explicit one-hour execution timeout.
The wrapper polls that command through a bounded terminal-state check.

If installation fails, the activator stops partial new services.
It keeps the failed runtime and archive below `/opt` with a dated `failed` name.
It then restores the old runtime and runs the old installer.
That installer restores host files, plugin files, units, configuration, and world services.

Exit status `20` means that activation failed and rollback completed.
Exit status `21` means that activation and rollback both failed.

An older stack can lack the `RelayDomain` parameter.
For that stack only, set an explicit `BIBITES_RELAY_DOMAIN` before the update.
The wrapper validates the fallback name before it sends a remote command.

## Backups and recovery

`backup-template.yaml` creates daily encrypted EBS snapshots for tagged volumes.
Its default policy keeps seven snapshots.

Record the latest checked snapshot in private operations storage.
Run a controlled reboot or replacement check before you depend on the recovery path.

After a recovery, make sure that each sidecar reports `reason=reclaimed`.
Make sure that each world uses its original peer identity and position.

Do not delete a retained volume until a checked final snapshot exists.
Resolve the exact volume and snapshot identifiers from private inventory.

## GPU broadcaster safety gate

`broadcast-template.yaml` defines a Spot GPU host with no inbound security-group rules.
The retained interface is limited to `slot-1`.

The deployment wrapper performs read-only checks for these properties:

- The instance supports `x86_64` and reports an NVIDIA GPU.
- The Spot quota covers the selected type's default vCPU count.
- The relay and RTMP destinations use approved private routes.
- The publish parameter is a default-key `SecureString`.
- The staged manifest disables `slot-1`.
- The snapshot is complete, encrypted, and owned by the approved account.

The route check uses the longest matching AWS route.
It rejects a more specific blackhole, internet gateway, NAT gateway, or network interface route.
Approved targets are local, peering, transit gateway, and private virtual gateway routes.
The check fails closed if the selected route table contains a managed prefix-list route.
The current preflight does not resolve the networks inside managed prefix lists.

The wrapper reads parameter metadata only.
It does not decrypt or copy the publish password.

The current publisher is disabled.
FFmpeg needs the authenticated RTMP URL as a process argument in this design.
That behavior exposes the password to process inspection.

`deploy-broadcast.sh` fails before it sends a remote command or changes CloudFormation.
The host installer and stream command also fail closed.
There is no unsafe override.

Before enablement, implement a transport that keeps the password out of arguments, status, and logs.
After the gate is removed, the wrapper runs `source-world-stopped.sh`.
That proof checks the game and sidecar services independently.
Both services must be inactive and disabled.

Review [`docs/live-broadcast.md`](../../docs/live-broadcast.md) before work on this gate.

If GPU quota is not available, use the separate
[`deploy/local-broadcast/`](../../deploy/local-broadcast/README.md) fallback.
It runs one new map world on a local Windows NVIDIA GPU.
That world enrolls its own identity, so it never copies the `slot-1` credential this stack moves.

## Cost checks

Do not use a quote recorded in this repository.
Read current Spot history, storage prices, public IPv4 charges, transfer prices, and snapshot costs.

Example Spot query:

```sh
aws --profile "$AWS_PROFILE" --region "$AWS_REGION" \
  ec2 describe-spot-price-history \
  --instance-types "$BIBITES_INSTANCE_TYPE" \
  --product-descriptions Linux/UNIX \
  --start-time "$(date -u +%FT%TZ)" --max-items 20
```

Store the quote time, monthly forecast, and budget thresholds in private operations storage.
