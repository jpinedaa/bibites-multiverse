# AWS Lightsail handoff

This checklist applies the public deployment kit to a new Lightsail instance.
It does not describe the state of a live service.

Keep account identifiers, instance names, addresses, costs, alarms, and execution receipts in private operations storage.

## Entry conditions

Before you start, make sure that these conditions are true:

- The operator selected the AWS account and region.
- The operator checked current pricing and account eligibility.
- `SIZING.md` supports the selected memory, disk, and transfer limits.
- The service domain is registered.
- The retention rule and service period are approved.
- A person owns the alert channel and backup policy.

Use a limited AWS identity for routine work.
Do not use a root access key for automation.

## AWS resources

Create these resources in this order:

1. Create an Ubuntu instance with the approved bundle.
2. Attach a Lightsail static IP.
3. Point the DNS records at the static IP.
4. Open provider firewall rules for SSH, HTTP, and HTTPS.
5. Enable a provider alarm for failed instance status checks.
6. Enable an off-host snapshot policy.

Attach the static IP before you create the DNS record.
The default instance address can change after a stop and start.

The host firewall is separate from the Lightsail firewall.
Both firewalls must permit the required traffic.

## Install the kit

Build and copy the binaries from the development machine:

```sh
deploy/ship.sh --host <static-ip-or-ssh-alias>
scp -r deploy <user>@<host>:/home/<user>/multiverse-kit
```

On the instance, install and edit the parameter file:

```sh
getent group multiverse >/dev/null || sudo groupadd --system multiverse
sudo install -d -m 0750 -o root -g multiverse /etc/multiverse
sudo install -o root -g multiverse -m 0640 \
  /home/<user>/multiverse-kit/deploy.env.example \
  /etc/multiverse/deploy.env
sudo editor /etc/multiverse/deploy.env
```

Do not copy a completed parameter file back into the repository.
Do not place a join string or alert capability in shell history.

## Dry run

Run the dry run before any provisioning run:

```sh
sudo /home/<user>/multiverse-kit/provision.sh --dry-run
```

Make sure that the output identifies the intended domain and addresses.
Check public DNS from a different machine.

If DNS is incorrect, stop before certificate issuance.
Correct the record and wait for its TTL.

## Provision and check

Run the provisioning script:

```sh
sudo /home/<user>/multiverse-kit/provision.sh
```

Then run these checks:

```sh
sudo /opt/multiverse/deploy/provision.sh --only verify
sudo -u multiverse /opt/multiverse/deploy/monitor.sh --test
sudo -u multiverse /opt/multiverse/deploy/monitor.sh --verbose
sudo systemctl start multiverse-backup.service
sudo -u multiverse /opt/multiverse/deploy/backup.sh --list
```

The alert test must reach the assigned person.
The archive status must show an active relay subscription.
The backup must include `ring.json`, `peers.json`, and checksums.

Check the website and relay path from outside the instance.
An on-host `/etc/hosts` entry can make an internal check pass when public DNS fails.

## Reboot gate

Use a controlled reboot before the service accepts participants.
After the reboot, make sure that nginx, the relay, and the archive are active.

Make sure that the archive reconnects to the relay.
Run the full monitor check again.

Do not use the first participant session as the reboot test.

## Participant gate

Before you enable automatic enrollment or issue a private-map join string, publish these service
facts:

- The announced period.
- The retention rule and genome horizon.
- The restart effects.
- The wind-down commitments.
- The data that the map publishes.

Follow `ANNOUNCEMENT.md`, `RESTART-POLICY.md`, and `WIND-DOWN.md`.

Set finite automatic-enrollment limits. Test the HTTPS route without creating a credential.
Back up the verifier store after automatic or manual enrollment begins.

## Private handoff record

Record these facts outside the public repository:

- The AWS account, region, instance, bundle, and static IP.
- The DNS records and certificate names.
- The deployed public commit.
- The current parameter choices without secret values.
- The alarm, snapshot, backup, and restore state.
- Verification output and deployment timestamps.
- The current monthly forecast and budget thresholds.

This public checklist stays stable across deployments.
