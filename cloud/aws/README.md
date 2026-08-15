# AWS cloud-world kit

This directory contains reusable infrastructure for headless Bibites worlds on AWS.
It does not contain a live inventory, current quote, source-machine migration, or deployment receipt.

The main stack uses one persistent EC2 Spot host and one retained EBS data volume.
Each world has a separate game directory, configuration root, sidecar journal, and credential file.

The optional broadcast stack runs one graphical world on a GPU host.
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
- `jq`, `unzip`, `tar`, and `sha256sum`.
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
Do not place a complete join string in S3, a manifest, or a command argument.

## Build artifacts

Set paths to authorized local archives:

```sh
export BIBITES_GAME_ZIP=/protected/path/TheBibites-Linux.zip
export BIBITES_BEPINEX_ZIP=/protected/path/BepInEx-linux-x64.zip
./cloud/aws/build-artifacts.sh
```

The build script checks the supported game assembly and creates a content-addressed runtime archive.

## Stage artifacts

Set the private manifest and save directory:

```sh
export BIBITES_MANIFEST_FILE=/protected/deployment/worlds.json
export BIBITES_SAVE_DIR=/protected/deployment/saves
./cloud/aws/stage-artifacts.sh
```

The stage script creates a private encrypted S3 bucket when necessary.
It blocks public access and uploads the manifest, saves, game archive, BepInEx archive, and runtime.

Record the bucket and object prefix in private operations storage.

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

The sidecar journal is durable before an acknowledgment.
Shutdown does not create that durability.

CAUTION: Do not run two copies from one save and credential.
The copies can create different descendants from one checkpoint.

CAUTION: Do not import a sidecar journal from another map.
Old custody records can target a different topology.

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
```

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

## Backups and recovery

`backup-template.yaml` creates daily encrypted EBS snapshots for tagged volumes.
Its default policy keeps seven snapshots.

Record the latest checked snapshot in private operations storage.
Run a controlled reboot or replacement check before you depend on the recovery path.

After a recovery, make sure that each sidecar reports `reason=reclaimed`.
Make sure that each world uses its original peer identity and position.

Do not delete a retained volume until a checked final snapshot exists.
Resolve the exact volume and snapshot identifiers from private inventory.

## Optional GPU broadcaster

`broadcast-template.yaml` defines a Spot GPU host with no inbound security-group rules.
The host starts one graphical world from an approved EBS snapshot.

Xorg uses the NVIDIA GPU.
FFmpeg uses NVENC for the shared H.264 stream.

The deployment wrapper requires an existing publish-password parameter.
It does not read or copy the secret value.

Review [`docs/live-broadcast.md`](../../docs/live-broadcast.md) before deployment.

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
