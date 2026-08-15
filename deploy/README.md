# `deploy/` hosting kit

This directory contains the reusable kit for a public Bibites Multiverse relay and archive.
It does not contain a record of any live deployment.

Keep these records outside the public repository:

- Cloud account numbers and resource identifiers.
- Current addresses, costs, quotas, and alarm state.
- Deployment receipts, incident notes, and operator contact details.
- Join strings, alert URLs, private keys, and other secret values.

## Files

| File | Purpose |
|---|---|
| `deploy.env.example` | Parameters for one deployment. Copy it outside the repository before use. |
| `provision.sh` | Installs and configures a host in named, repeatable phases. |
| `ship.sh` | Builds Linux binaries and copies them to a host. |
| `issue-join.sh` | Creates participant credentials during a planned relay restart. |
| `monitor.sh` | Checks services, capacity, certificates, backups, and map health. |
| `backup.sh` | Creates local identity and archive backups. It also prints recovery guidance. |
| `install-stream-origin.sh` | Installs an optional private RTMP and loopback HLS origin. |
| `tls-deploy-hook.sh` | Installs a renewed certificate and reloads nginx. |
| `test-front-door.sh` | Renders and checks the nginx configuration. |
| `systemd/` | Service and timer units for the relay, archive, monitor, and backup. |
| `nginx/` | HTTP challenge and shared HTTPS front-door templates. |
| `SIZING.md` | Stable capacity measurements and sizing formulas. |
| `RESTART-POLICY.md` | Generic restart rules and participant effects. |
| `WIND-DOWN.md` | Public service commitments for a planned ending. |
| `ANNOUNCEMENT.md` | Service communication policy. The historical announcement copy is not public guidance. |
| `HANDOFF-domain.md` | Generic domain and DNS handoff checklist. |
| `HANDOFF-lightsail.md` | Generic Lightsail handoff checklist. |

## Configuration boundary

Copy `deploy.env.example` to `/etc/multiverse/deploy.env` on the host.
Keep the installed file at mode `0640` with owner `root:multiverse`.

The example contains safe placeholders and conservative defaults.
It does not contain the values for a live service.

The following values identify one deployment:

- `MV_DOMAIN` and `MV_CERT_EXTRA_NAMES`.
- `MV_ACME_EMAIL`.
- `MV_RETENTION` and `MV_ARCHIVE_GENOME_HORIZON`.
- `MV_PERIOD_START` and `MV_PERIOD_END`.
- `MV_ALERT_KIND`, `MV_ALERT_URL`, and `MV_ALERT_COMMAND`.
- `MV_PUBLIC_ENROLLMENT` and its total, per-address, and window limits.
- The optional stream-ingest address and source CIDR.

Never store the completed file in Git.
Store secret values in a secret manager or in protected files on the host.

## Architecture

nginx owns the public HTTP and HTTPS ports.
It sends `/contract-b/` WebSocket traffic to the relay on loopback.
It sends only `POST /api/enroll` to the relay's public enrollment handler.
It sends website requests to the archive on loopback.
It sends `/stream/` requests to the optional HLS origin on loopback.

The relay trusts a forwarded client address only from a loopback connection.
The archive HTTP listener must also remain on loopback.
These rules keep the application listeners off the public network.

The default certificate flow uses ACME HTTP-01 through nginx.
DNS-01 needs a provider plugin and a protected DNS credential.
The provisioning script refuses DNS-01 until an operator configures that integration.

## Before provisioning

Complete these tasks before you run a mutating command:

1. Select the cloud account and region.
2. Record the approved account identifier in your private operations record.
3. Create the instance and attach a stable public address.
4. Point the service name at that stable address.
5. Open the provider firewall for SSH, HTTP, and HTTPS.
6. Select the retention rule and announced service period.
7. Configure an alert channel that reaches a person.
8. Configure an off-host backup or snapshot policy.
9. Review `SIZING.md`, `RESTART-POLICY.md`, and `WIND-DOWN.md`.

Check DNS from a machine outside the instance.
An `/etc/hosts` entry on the instance can hide an incorrect public record.

## Install a host

Run these commands from the development machine:

```sh
deploy/ship.sh --host <host-or-ssh-alias>
scp -r deploy <user>@<host>:/home/<user>/multiverse-kit
```

Run these commands on the instance:

```sh
getent group multiverse >/dev/null || sudo groupadd --system multiverse
sudo install -d -m 0750 -o root -g multiverse /etc/multiverse
sudo install -o root -g multiverse -m 0640 \
  /home/<user>/multiverse-kit/deploy.env.example \
  /etc/multiverse/deploy.env
sudo editor /etc/multiverse/deploy.env

sudo /home/<user>/multiverse-kit/provision.sh --dry-run
sudo /home/<user>/multiverse-kit/provision.sh
```

Read every warning from the dry run.
Do not request a production certificate until public DNS is correct.

## First verification

Run these checks before you issue a participant credential:

```sh
sudo -u multiverse /opt/multiverse/deploy/monitor.sh --test
sudo -u multiverse /opt/multiverse/deploy/monitor.sh --verbose
sudo systemctl start multiverse-backup.service
sudo -u multiverse /opt/multiverse/deploy/backup.sh --list
sudo /opt/multiverse/deploy/provision.sh --only verify
```

Then make sure that all required services start after a controlled reboot.
Check the public website and relay path from another network.

The alert test must reach its intended recipient.
The backup list must contain the identity files and their checksums.
The archive status must show an active relay subscription.

## Public enrollment and manual join issuance

Each public installer creates a secret and installation UUID locally. The installer sends these
values to `POST /api/enroll` over HTTPS. The relay stores a verifier. It returns the derived
identity and advertised WSS address. The endpoint is disabled by default in the relay binary.
The deployment parameter file enables it with finite limits.

Set `MV_PUBLIC_ENROLLMENT=0` and re-run the envfiles phase to stop new automatic identities. This
does not revoke an existing credential. Review
[`contracts/public-enrollment.md`](../contracts/public-enrollment.md) before changing a limit.

The verification phase checks that GET cannot reach enrollment. Do not send a synthetic POST to a
live service: a valid POST creates durable identity state. Test creation with a real installer and
keep that installed identity.

Private maps and manually named identities still use `issue-join.sh`.

Credential creation requires a planned relay restart.
Collect the approved peer identifiers before the restart.
Then create the credentials in one batch.

```sh
sudo /opt/multiverse/deploy/issue-join.sh <peer-id> [<peer-id> ...]
```

Transfer each join string through an approved private channel.
Delete temporary secret copies after the participant applies them.
Back up the verifier store after issuance.

Do not publish join strings in issues, logs, documentation, or command transcripts.

## Public service commitments

Give each participant the following information before they join:

- The service period and the rule for extensions or early closure.
- The retention rule for the ledger and genome blobs.
- The data that the map publishes about a world.
- The normal effects of relay and archive restarts.
- The wind-down timeline and the disposition of service records.

Use `ANNOUNCEMENT.md` as the communication policy.
Use `WIND-DOWN.md` as the public commitment.
Keep channel-specific outreach records in private operations storage.

## Changes to a live service

Run `provision.sh` again to apply a changed parameter file or deployment kit.
The script installs files, but it does not make every restart decision for you.

Read `RESTART-POLICY.md` before a relay, archive, or host restart.
Batch archive changes because replay time grows with the ledger.

Use `test-front-door.sh` after an nginx template change.
Use the provisioning verification phase after any host change.

## Public and private records

The public repository owns reusable behavior and stable engineering evidence.
A private operations record must own the mutable state of each deployment.

The private record needs at least these items:

- The deployed public commit.
- Resource identifiers and current addresses.
- The completed parameter values, without secret values.
- Current costs, budgets, quotas, alarms, and backup state.
- Deployment receipts, incidents, and recovery evidence.
- Credential names, storage locations, owners, and rotation dates.

Public documents must not link to private records.
Private records can link to a public file at a fixed commit.

## Static checks

Run these checks after a public kit change:

```sh
bash -n deploy/*.sh
deploy/test-front-door.sh
```

Run `shellcheck` when it is available.
Run `systemd-analyze verify` against the units on a compatible Linux host.
Run the repository link and release checks before publication.
