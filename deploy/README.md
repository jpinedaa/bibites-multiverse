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
| `monitor.sh` | Checks services, capacity, certificates, backups, map health, and the monthly data-transfer allowance. |
| `health-snapshot.sh` | Records one numeric reading of the live map. It keeps the numbers and decides nothing. |
| `service-host-sample` | Records one sample of this host: CPU, load, memory, disk, per-unit state, and TCP counters. |
| `backup.sh` | Creates local identity and archive backups. It also prints recovery guidance. |
| `install-stream-origin.sh` | Installs an optional private RTMP and loopback HLS origin. |
| `tls-deploy-hook.sh` | Installs a renewed certificate and reloads nginx. |
| `test-front-door.sh` | Renders and checks the nginx configuration. |
| `test-units.sh` | Checks the systemd units, including the archive's start-time dependencies. |
| `test-monitor.sh` | Drives the monitor's data-transfer arithmetic against fake counters and a fake clock. |
| `local-broadcast/` | Runs the optional Windows GPU broadcast fallback. |
| `systemd/` | Service and timer units for the relay, archive, monitor, backup, and host sampler. |
| `nginx/` | HTTP challenge and shared HTTPS front-door templates. |
| `www/announcements/` | The announcements page nginx serves from disk, and the seed for its notices file. |
| `SIZING.md` | Stable capacity measurements and sizing formulas. |
| `RESTART-POLICY.md` | Generic restart rules and participant effects. |
| `WIND-DOWN.md` | Public service commitments for a planned ending. |
| `ANNOUNCEMENT.md` | Service communication policy. The historical announcement copy is not public guidance. |
| `HANDOFF-domain.md` | Generic domain and DNS handoff checklist. |
| `HANDOFF-lightsail.md` | Generic Lightsail handoff checklist. |

[`docs/observability.md`](../docs/observability.md) is the measurement standard that the monitor,
the two samplers, and the health snapshot belong to.
It states what is measured, at what interval, where each reading lands, and who reads it.

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
- `MV_BUNDLE` and `MV_TRANSFER_ALLOWANCE_GB`, which must state the same bundle.
- `MV_PUBLIC_ENROLLMENT` and its total, per-address, and window limits.
- The optional stream-ingest address and source CIDR.
- Homepage values for the public landing page links: `MV_HOMEPAGE_RELEASE`,
  `MV_HOMEPAGE_REPO`, and `MV_HOMEPAGE_GAME_VERSION`.

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

The verbose run prints one line for each check.
Read the three that watch money rather than availability:

- `transfer` compares month-to-date transfer, and the last 24 closed hours projected over a month,
  against `MV_TRANSFER_ALLOWANCE_GB`. It reads `/proc/net/dev` on the billed interface and needs no
  cloud credential. It says `warming up` for its first six hours, which is correct and not a fault.
- `transfer-rate` reports the last closed hour and warns after `MV_TRANSFER_HOURLY_RUNS`
  consecutive hours above `MV_TRANSFER_HOURLY_GB`.
- `hosts-pin` checks that `/etc/hosts` still pins `MV_DOMAIN` to `127.0.0.1`. Without that line the
  archive's subscription to the relay leaves the host and returns over the billed interface.

The transfer figures use the provider's GB of 2^30 bytes, counted in both directions.

## Measurement

`monitor.sh` answers whether anything is wrong now.
It compares each reading with a threshold and keeps only the severity.
The two tools below keep the reading instead, because a change to a live service asks a different
question: is the service worse than it was fifteen minutes ago.

`service-host-sample` records one sample of this host as a single JSON line.
It runs unprivileged, reads `/proc` and `systemctl show` only, and reports every value it cannot
read as unknown instead of zero.
The provisioning script installs neither the sampler nor its timer, so install both by hand:

```sh
sudo install -m 0755 -o root -g root \
  /home/<user>/multiverse-kit/service-host-sample \
  /opt/multiverse/deploy/service-host-sample
sudo -u multiverse /opt/multiverse/deploy/service-host-sample --stdout | jq

sudo install -d -m 0750 -o multiverse -g multiverse /var/lib/multiverse/metrics
sudo install -m 0644 -o root -g root \
  /opt/multiverse/deploy/systemd/multiverse-host-sample.service \
  /opt/multiverse/deploy/systemd/multiverse-host-sample.timer \
  /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now multiverse-host-sample.timer
sudo tail -n 3 /var/lib/multiverse/metrics/service-host.jsonl | jq
```

Create the metrics directory before the timer runs.
The unit confines its writes to that path, and it cannot start while the path is absent.

The timer samples every minute, and one sample is about a kilobyte, so the file grows by roughly
1.5 MB each day.
Its TCP counters are cumulative, so compare two samples instead of reading one.

`health-snapshot.sh` reads the published map and prints one JSON record for each sample.
It only sends GET requests, so it is safe against a live service and safe from a workstation.

```sh
deploy/health-snapshot.sh --url https://<service-name> | jq

sudo -u multiverse /opt/multiverse/deploy/health-snapshot.sh \
  --url http://127.0.0.1:8796 --label before-change
sudo -u multiverse /opt/multiverse/deploy/health-snapshot.sh \
  --watch 15m --markdown --label after-change
```

The first form runs from a workstation and reads the public front door.
The other two run on the host and read the archive on loopback.
The last form samples a window and prints the table rows a deployment record needs.
One reading before and one after cannot show a fault that resolves between them.

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

## Post an announcement

`https://<MV_DOMAIN>/announcements/` is the participant-facing notice channel.
nginx serves it from `/var/www/announcements/` on disk, and not from the archive.
That is deliberate: a notice usually announces that the archive is about to stop, and an archive
route would take the notice offline for exactly the window it describes.
The page keeps answering while the archive is stopped, replaying its ledger, or failed.
It stops only when the whole host does.

The page is two files with two different owners:

| Path | Owner | Overwritten by `provision.sh` |
|---|---|---|
| `/var/www/announcements/index.html` | The kit. Rendered from `www/announcements/index.html`. | Yes, on every `nginxfront` run. |
| `/var/www/announcements/notices.html` | The operator. | No. It is seeded once, only when it is absent. |

nginx joins them with a server-side include at request time.
Posting a notice writes one file. It needs no rebuild, no restart, and no reload.

One notice is one `<article class="notice">` fragment. Newest first, so post at the top:

```sh
sudo sh -c 'cat - /var/www/announcements/notices.html > /var/www/announcements/.notices.new \
  && chmod 0644 /var/www/announcements/.notices.new \
  && mv /var/www/announcements/.notices.new /var/www/announcements/notices.html' <<'EOF'
<article class="notice" id="n-2026-01-01-example">
  <p class="when"><time datetime="2026-01-01T09:00Z">2026-01-01 09:00 UTC</time> · Planned maintenance</p>
  <h2>One short headline</h2>
  <p>The affected service, the window, the expected duration, and the participant action.</p>
  <!-- followup:2026-01-01-example -->
</article>
EOF
```

Keep the `followup` comment. It is the anchor for the line you add after the event:

```sh
sudo sed -i 's|<!-- followup:2026-01-01-example -->|<p class="followup"><b>Update 2026-01-01 09:25 UTC.</b> Service returned at 09:19 UTC. The outage lasted 11 minutes.</p>|' \
  /var/www/announcements/notices.html
```

Check the published result before you stop:

```sh
curl -fsS https://<MV_DOMAIN>/announcements/ | grep -c '<article class="notice"'
```

The page caches for 30 seconds, so a correction is visible within that.

`ANNOUNCEMENT.md` governs what a notice may say.
State the affected service, the expected duration, and the participant action.
Never imply that archive records missed during an outage can be reconstructed.
Never put a join string, an alert URL, a private address, or a resource identifier in a notice.

Record the date, channel, audience, and exact public link of each notice in private operations
storage, as `ANNOUNCEMENT.md` requires.

## Changes to a live service

Run `provision.sh` again to apply a changed parameter file or deployment kit.
The script installs files, but it does not make every restart decision for you.

Read `RESTART-POLICY.md` before a relay, archive, or host restart.
Batch archive changes because replay time grows with the ledger.

Use `test-front-door.sh` after an nginx template or announcements-page change.
Use `test-units.sh` after a systemd unit change.
Use `test-monitor.sh` after a `monitor.sh` change.
Use the provisioning verification phase after any host change.

Use `health-snapshot.sh --watch` across the change window.
The verification phase reports that each check passed, and a service can be measurably worse with
every check still passing.

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
bash -n deploy/*.sh deploy/service-host-sample
deploy/test-front-door.sh
deploy/test-units.sh
deploy/test-monitor.sh
```

`test-monitor.sh` needs no root, no network and no host.
It runs `monitor.sh --only transfer` and `--only hosts-pin` against fake `/proc` files, a fake
`/etc/hosts` and a fixed clock, in a temporary state directory.

Run `shellcheck` when it is available.
Run `systemd-analyze verify` against the units on a compatible Linux host.
Run the repository link and release checks before publication.
