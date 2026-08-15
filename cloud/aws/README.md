# AWS cloud worlds

This kit runs six headless Bibites worlds on one persistent EC2 Spot host. Each world has a separate game folder and data root.

The host connects to the public relay through Lightsail VPC peering. The relay connection uses the private address in the same Availability Zone.

The host currently imports these worlds:

| Cloud world | Source | State |
|---|---|---|
| `M4-Slot1` through `M4-Slot5` | The five Windows worlds on the main PC | Running on the cloud host |
| `M4-Slot6` | The Windows world on the laptop | Running on the cloud host |

The live host is EC2 instance `i-0a6ee98a24f27bfae`. Its retained data volume is
`vol-0d51c430a70521630`. On 2026-08-15 UTC, all six imported worlds passed the strict readiness
test. The first five worlds also passed a controlled reboot. The test requires an active game,
an active mod, a relay grant, and an applied time scale of `100` for every world.

The cloud keeps the exact five source checkpoints as timestamped saves. Their SHA-256 values match
the local migration copies. Each world also wrote a newer save before the controlled reboot.

The migration imports game saves only. It does not import the old LAN sidecar journals.

The old journals contain custody records for the retired LAN relay. A replay into the public map can route old organisms into a different topology.

Each cloud world gets a new public-map credential. The world save and evolutionary history remain unchanged.

The live-broadcast components use a separate GPU host. The page and stream
origin are live. AWS has not granted the required G-instance quota yet.
See [`docs/live-broadcast.md`](../../docs/live-broadcast.md).

## Cost and capacity

The default host is `m6a.2xlarge` in `us-east-1a`. It has 8 vCPUs and 32 GiB of memory.

The live Spot quote was `$0.1411/hour` on 2026-08-15 UTC. The expected monthly base is approximately `$114`:

| Item | Monthly cost |
|---|---:|
| Spot compute at the quoted price | `$103.00` |
| 80 GB encrypted gp3 data volume | `$6.40` |
| 12 GB encrypted gp3 root volume | `$0.96` |
| One public IPv4 address | `$3.65` |
| Private S3 artifact storage | Less than `$0.01` |

Daily EBS snapshots add variable incremental storage cost and are not in the base total. Network
traffic to the relay uses the private same-AZ route.

The same live quote query returned `$0.1515/hour` for `c7a.2xlarge`, `$0.1629/hour` for
`c7i.2xlarge`, and `$0.2059/hour` for `m7a.2xlarge`. The selected host was the least expensive of
these 8-vCPU x86 choices and has twice the memory of both compute-optimized choices.

After the five-world restart, the host used approximately 3 GiB of memory and no swap. The games
used approximately six vCPUs in total. Achieved time scale changed with each population and was
approximately x15 through x32 in the recovery sample. The configured x100 value is an upper target,
not a promise that every population can simulate at x100.

Six `c7a.large` hosts cost approximately `$190/month` with six addresses and six disk sets. The single host removes approximately 40% of that cost.

The consolidated host is one failure domain. A Spot interruption stops all worlds until the same instance restarts.

The Multiverse routes around the dark worlds during this interval. After Spot capacity returns, the persistent request restarts the same instance.

Run this command before a purchase decision or instance-type change:

```sh
aws --profile bibites-multiverse --region us-east-1 ec2 describe-spot-price-history \
  --instance-types m6a.2xlarge --product-descriptions Linux/UNIX \
  --start-time "$(date -u +%FT%TZ)" --max-items 20
```

## Data layout

The encrypted 80 GB volume mounts at `/srv/bibites`. CloudFormation retains this volume after stack deletion.

```text
/srv/bibites/
├── base/game/                 one verified Linux game and BepInEx base
├── bin/                       the sidecar
├── worlds/slot-N/
│   ├── game/                  one game folder per process
│   ├── config/                Unity saves and preferences
│   ├── sidecar/               new public-map journal and identity
│   ├── logs/
│   ├── peer-secret.txt        mode 0600, loaded from Parameter Store
│   └── world.env              mode 0600
└── metrics/performance.jsonl  five-minute local samples
```

The game folders use hard links for immutable game files. Each folder has a separate BepInEx log path.

## Safety rules

Use only Jorge Pineda's personal AWS account, `663615031964`, for this project. Use the
`bibites-multiverse` profile. The default profile points to DERSec and is prohibited.

Before any mutating command, make sure that the account is correct:

```sh
aws --profile bibites-multiverse sts get-caller-identity --query Account --output text
```

The command must print `663615031964`.

CAUTION: Do not start an old source world after its cloud copy starts. Two running copies create different descendants from one checkpoint.

CAUTION: Do not copy the old sidecar data to this host. Those journals belong to another relay and map.

CAUTION: Do not delete the retained EBS volume during recovery. This volume contains all saves and new custody journals.

The security group has no inbound rules. Use AWS Systems Manager Session Manager for administration.

The join secrets exist in encrypted Parameter Store. They do not occur in user data, process arguments, logs, or the manifest.

## Initial deployment

The commands use the `bibites-multiverse` AWS profile and the `us-east-1` Region.

1. Stop the five games on the main PC.

2. Capture the five final saves.

   ```sh
   ./cloud/aws/migrate-pc-saves.sh
   ```

3. Stop slot 6 on the laptop.

   ```powershell
   .\stop-slot6.ps1
   ```

4. Copy this file from the laptop to the main PC:

   ```text
   %USERPROFILE%\AppData\LocalLow\The Bibites\The Bibites\Savefiles\M4-Slot6.zip
   ```

5. Import the laptop save on the main PC.

   ```sh
   ./cloud/aws/import-slot6-save.sh /path/to/M4-Slot6.zip
   ```

6. Build the runtime and manifest.

   ```sh
   ./cloud/aws/build-artifacts.sh
   ./cloud/aws/make-manifest.sh
   ```

7. Upload the private artifacts.

   ```sh
   ./cloud/aws/stage-artifacts.sh
   ```

8. Put each join string in `cloud/aws/private/slot-N.join` with mode `0600`.

9. Deploy the host.

   ```sh
   ./cloud/aws/deploy-host.sh
   ```

10. Enable daily snapshots.

    ```sh
    ./cloud/aws/deploy-backups.sh
    ```

11. Make sure that every imported world is live.

    ```sh
    ./cloud/aws/verify-host.sh
    curl -fsS https://bibitesmultiverse.com/api/status | jq '.slots'
    ```

If Lightsail VPC peering is off, the deployment script enables it. Peering has no hourly charge.

## Add slot 6 after the host starts

The current host started with worlds 1 through 5. Run this procedure from a WSL checkout on the
laptop. The command uses the game and runtime assets that are already in S3. It does not require a
local build.

1. Close slot 6 cleanly. Wait until the save finishes. Do not force-stop the game.

2. Configure the `bibites-multiverse` AWS profile. Confirm that it selects account `663615031964`.

3. Run the migration with the WSL path to the save.

   ```sh
   ./cloud/aws/migrate-laptop-slot6.sh \
     /mnt/c/Users/USERNAME/AppData/LocalLow/The\ Bibites/The\ Bibites/Savefiles/M4-Slot6.zip
   ```

The command refuses to continue while a local Bibites process runs. It tests the save archive,
checks the AWS account and slot credential, and verifies that the cloud has the expected five-world
manifest. It then uploads the save, adds slot 6, synchronizes the host, and waits until all six worlds
are live on the public map.

The command is safe to retry with the same save. It never replaces a different cloud save. After it
succeeds, leave the laptop copy off to prevent two histories from the same checkpoint.

## Runtime behavior

Each sidecar starts before its game. The game starts only after the relay grants the sidecar a slot.

The game runs with `-batchmode -nographics`. One shared Xvfb service supplies the X11 backend that
the native Linux game requires. Unity still uses its null graphics device, so the host does not need
a GPU.

The target time scale is `100`. An initialization service applies the value twice after each world load.

Each world saves every 10 minutes and keeps six copies. A Spot interruption also sends `SIGTERM` to all games in parallel.

The interruption watcher waits for save-on-quit. Then it stops the sidecars and flushes the file system before EC2 stops the host.

The sidecar journals are durable before acknowledgements. Shutdown does not create their durability.

## Live broadcast host

`broadcast-template.yaml` defines one `g4dn.xlarge` Spot host. It uses the
official AWS NVIDIA-driver AMI and has no inbound security-group rules.

The host starts one graphical world from a retained EBS snapshot. Xorg uses the
NVIDIA GPU, and FFmpeg uses NVENC for the `1280x720` H.264 stream.

`deploy-broadcast.sh` refuses deployment when the G-instance quota is less than
4 vCPUs. It also requires a completed snapshot ID.

CAUTION: Stop and disable the source world before you make that snapshot. Do
not run the source and GPU copies at the same time.

## Operations

Start a Session Manager shell:

```sh
instance_id=$(aws --profile bibites-multiverse --region us-east-1 \
  cloudformation describe-stacks --stack-name bibites-cloud-worlds \
  --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' --output text)
aws --profile bibites-multiverse --region us-east-1 ssm start-session --target "$instance_id"
```

Show all world states:

```sh
sudo bibites-cloud-status
sudo bibites-cloud-status --json | jq
```

Read one world log:

```sh
sudo tail -n 100 /srv/bibites/worlds/slot-1/logs/sidecar.log
sudo tail -n 100 /srv/bibites/worlds/slot-1/game/BepInEx/LogOutput.log
```

Read the performance record:

```sh
sudo tail -n 12 /srv/bibites/metrics/performance.jsonl | jq
```

Install a newly staged runtime without replacement of the EC2 instance:

```sh
./cloud/aws/build-artifacts.sh
./cloud/aws/stage-artifacts.sh
./cloud/aws/update-runtime.sh
./cloud/aws/verify-host.sh
```

CAUTION: Do not run `deploy-host.sh` on the live stack only to update scripts. A launch-template
change can replace the instance. Use `update-runtime.sh` for an in-place runtime update.

If sustained swap use is more than zero, move to a larger memory size or split the worlds across two hosts.

## Backups

The `bibites-cloud-worlds-backups` stack takes one encrypted snapshot each day at 06:00 UTC. It
keeps seven snapshots. The post-migration checkpoint is `snap-0dbb7fcea96bb958c`.

The in-game save schedule and save-on-quit are the first recovery layer. EBS snapshots protect the
complete volume, which also contains the sidecar custody journals and world configuration.

## Recovery checks

Use an EC2 reboot for a controlled recovery test. Do not terminate the instance.

```sh
aws --profile bibites-multiverse --region us-east-1 ec2 reboot-instances \
  --instance-ids "$instance_id"
```

After the instance returns, run `verify-host.sh`. Each sidecar must report `reason=reclaimed` in its log.

This recovery test passed on 2026-08-15 UTC. All five worlds reclaimed their original slots and
reapplied the x100 target.

The data volume has `DeletionPolicy: Retain`. A stack deletion removes compute but keeps the volume and its monthly charge.

To retire the deployment, make a final snapshot first. Then make sure that the snapshot is usable.

After this test, delete the retained volume explicitly.
