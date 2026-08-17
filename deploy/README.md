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
| `deploy.sh` | The deployment sequence on the host: install the kit, prove what landed, install the binaries, prove those. It restarts nothing. |
| `ci-gate.sh` | The forced command for a CI deployment key. It accepts a fixed list of verbs and refuses everything else. |
| `issue-join.sh` | Creates participant credentials during a planned relay restart. |
| `restart-relay.sh` | Restarts the relay behind a peer gate, then proves the archive resubscribed before the first placement claim. Roughly 30 to 60 seconds. |
| `restart-archive.sh` | Runs the complete-record archive sequence, guarded by the archive-deploy hold and the replay-headroom verdict. Costs a full ledger replay. |
| `restart-lib.sh` | The half both restart scripts share: the peer gate, the waits, the proof, and the scratch directory its EXIT trap removes. Sourced, never run. |
| `monitor.sh` | Checks services, capacity, certificates, backups, map health, and the monthly data-transfer allowance. |
| `ce-reconcile.sh` | Reconciles the host's transfer counter against the invoice once a day. It runs on an operator machine, not on the host. |
| `health-snapshot.sh` | Records one numeric reading of the live map. It keeps the numbers and decides nothing. |
| `service-host-sample` | Records one sample of this host: CPU, load, memory, disk, per-unit state, and TCP counters. |
| `backup.sh` | Creates local identity and archive backups. It also prints recovery guidance. |
| `coldcopy.sh` | Copies each closed ledger segment to an object store, verifies it against the store, and writes the receipt without which the archive removes nothing. |
| `install-stream-origin.sh` | Installs an optional private RTMP and loopback HLS origin. |
| `viewers-presence.sh` | Publishes whether anyone is watching the broadcast, so the publisher can stop while nobody is. |
| `tls-deploy-hook.sh` | Installs a renewed certificate and reloads nginx. |
| `test-front-door.sh` | Renders and checks the nginx configuration. |
| `test-units.sh` | Checks the systemd units, including the archive's start-time dependencies. |
| `test-monitor.sh` | Drives the monitor's transfer, billing, hosts-pin, replay, record-layer and swap arithmetic against fake counters, a fake status document and a fake clock. |
| `test-coldcopy.sh` | Drives `coldcopy.sh` through a fake AWS CLI, including every way the store can disagree. It makes no API call and reads no credential. |
| `test-viewers-presence.sh` | Drives `viewers-presence.sh` against a fake access log, fake metrics and a fixed clock. |
| `test-ce-reconcile.sh` | Drives `ce-reconcile.sh` against a saved Cost Explorer response and a fake metric provider. It makes no API call. |
| `test-ci-gate.sh` | Drives `ci-gate.sh` through its verb allowlist, including the attempts to get past it. |
| `test-deploy.sh` | Checks `deploy.sh`'s kit listing digest against the method the deployment record defines. |
| `testdata/` | Saved Cost Explorer responses that `test-ce-reconcile.sh` parses, including a part-day response that pins the billing lag. |
| `local-broadcast/` | Runs the optional Windows GPU broadcast fallback. |
| `systemd/` | Service and timer units for the relay, archive, monitor, backup, host sampler, off-host segment copy, and viewer-presence signal. |
| `nginx/` | HTTP challenge and shared HTTPS front-door templates, and the front door's log rotation policy. |
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
- `MV_RETENTION`, `MV_ARCHIVE_GENOME_HORIZON` and `MV_LEDGER_WINDOW`.
- `MV_COLDCOPY` and `MV_COLDCOPY_URI`. The URI names a live resource, so it
  belongs on the host and in private operations storage and never in Git.
- `MV_PERIOD_START` and `MV_PERIOD_END`.
- `MV_ALERT_KIND`, `MV_ALERT_URL`, and `MV_ALERT_COMMAND`.
- `MV_BUNDLE` and `MV_TRANSFER_ALLOWANCE_GB`, which must state the same bundle.
- `MV_PUBLIC_ENROLLMENT` and its total, per-address, and window limits.
- The optional stream-ingest address and source CIDR.
- Homepage values for the public landing page links: `MV_HOMEPAGE_REPO` and
  `MV_HOMEPAGE_GAME_VERSION`. The release is not one of them. The page's
  download buttons address GitHub's `/releases/latest`, so a new release reaches
  the homepage without a deployment.

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

Later deployments stage a new kit beside this one. "Changes to a live service", "What a deployment
leaves on the host", says which copies to keep and when to remove the rest.

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

### `monitor.sh` exit codes

The exit code says whether the monitor ran. It does not say whether the service is well.

| Code | Meaning |
|---|---|
| `0` | The pass completed. `OK`, `WARN` and `CRIT` all exit `0`. |
| `1` | Only from `--test`, and only when the alert channel itself failed. |
| `2` | The pass could not start: no readable `/etc/multiverse/deploy.env`, or an argument the script does not accept. |

A `WARN` or a `CRIT` travels in the alert, in the `worst:` line the run prints, and in the `sev.*`
state under `MV_STATE`.
It does not travel in the exit code.
`multiverse-monitor.service` is a `Type=oneshot` driven by a five-minute timer, so a severity
carried in the exit code left the unit `failed` almost permanently while the monitor was doing
exactly its job.
`systemctl is-failed multiverse-monitor.service` then meant nothing, and a deployment pipeline
running under `set -o pipefail` broke on it.
A non-zero status now means the watcher could not run, which is the one fault no check of its own
can report.

To read the verdict from a script, parse the `worst:` line:

```sh
sudo -u multiverse /opt/multiverse/deploy/monitor.sh --quiet --verbose | awk '/^worst:/ { print $2 }'
```

`--quiet` sends no alert, but it still writes the shared `sev.*` state, so a pass run out of band
records what it saw as already alerted and the timer never announces it.
Point `MV_STATE` at a temporary directory when a script reads the monitor.
`restart-archive.sh` reads its replay gate exactly that way, from the per-check lines `--verbose`
prints.

## Operating deploys through CI

A deployment can be driven by a continuous-integration system instead of by hand.
The kit provides the two pieces that need to be the same every time.

`deploy.sh` is the sequence, and it is the same sequence either way:

```sh
sudo /opt/multiverse/deploy/deploy.sh --kit /path/to/staged-kit --binaries
```

It snapshots `/etc/multiverse/*.env`, installs the kit from the staged copy's own `provision.sh`,
proves every installed file is byte-identical to the file it came from, and refuses if a phase
rewrote an environment file — restoring the file first.
It installs the binaries only after those checks pass, and then proves each installed binary
against the artifact that was staged.
It restarts nothing.

`ci-gate.sh` is the forced command for the CI key.
Add it to the login account's `authorized_keys`:

```text
command="/opt/multiverse/deploy/ci-gate.sh",no-port-forwarding,no-agent-forwarding,no-pty,no-X11-forwarding ssh-ed25519 AAAA... mv-deploy-ci
```

The gate accepts these verbs and refuses everything else with exit code 3:

| Verb | What it does |
|---|---|
| `verify` | `provision.sh --only verify`, then reports whether the monitor is running. Changes nothing. |
| `phase <name>` | One provision phase, from a fixed list: `directories`, `binaries`, `nginxfront`, `streamorigin`, `verify`. |
| `kit-receive <sha>` | Reads a tar stream on stdin into a new directory under `~/ci-kits/`. |
| `kit-install <dir> [<digest>]` | `deploy.sh --kit <dir>`. |
| `binaries-install <dir> [<digest>]` | `deploy.sh --kit <dir> --binaries`. |
| `restart-relay [--dry-run] [<tag>]` | `restart-relay.sh`. |
| `restart-archive --dry-run` | `restart-archive.sh --dry-run`, and only ever the rehearsal. |
| `receipt` | Prints host facts for a deployment record. No secret is among them. |

Four rules the gate applies to all of them:

- **A dry run is the default.** Every mutating verb takes `--dry-run`, and it reaches
  `provision.sh`, which routes every mutation through one helper.
- **The archive is never restarted by CI.** `restart-archive` is accepted with `--dry-run` and with
  nothing else. An archive restart replays the whole ledger, costs the map a full relay outage for
  the length of the replay, and needs an operator's measured proof that the replay fits in memory.
  That is a judgement, and a scheduled job cannot make it. Run it by hand from `RESTART-POLICY.md`.
- **A hold stops it.** While `/run/lock/bibites-archive-deploy.HOLD-README` exists, every mutating
  verb is refused. `verify` and `receipt` stay available, so a check does not go red because a
  person is working. There is no override.
- **Mutations take the deployment lock.** `flock -n` on `/run/lock/bibites-archive-deploy.lock`,
  the same lock a hand-run deployment takes, so the two paths serialize against each other.

The gate writes one line per invocation to `/var/log/multiverse/ci-gate.log`:

```text
2026-08-16T12:00:00Z ALLOW verify
2026-08-16T12:00:04Z REFUSE not an allowed verb: rm
```

Set `MV_CI_DEPLOY_KEY_PUB` in `deploy.env` to the public half of that key.
The verify phase then checks that the key is still pinned to the forced command with all four
restrictions, and that `ci-gate.sh` is installed and root-owned.
That check exists because its failure is silent: the login account has passwordless sudo, so a CI
key that loses its forced command is root on the host and nothing about the service looks different.

Never put the private half of that key on the host or in this repository.

## Measurement

`monitor.sh` answers whether anything is wrong now.
It compares each reading with a threshold and keeps only the severity.
The two tools below keep the reading instead, because a change to a live service asks a different
question: is the service worse than it was fifteen minutes ago.

`service-host-sample` records one sample of this host as a single JSON line.
It runs unprivileged, reads `/proc`, `systemctl show` and the cgroup files only, and reports every
value it cannot read as unknown instead of zero.
Each unit carries `memoryBytes` from `MemoryCurrent` and, beside it, `anonBytes` and `fileBytes`
from the cgroup.
`MemoryCurrent` includes page cache, so it moves with file activity and is the wrong field for a
memory trend; `anonBytes` is the term that grows and the term the out-of-memory killer acts on.
The archive additionally carries `mainRssAnonBytes`, `mainVmHwmBytes` and `mainPid` from its main
process, because nothing else on the host remembers the replay peak or sees an operator's restart
between two samples.

`provision.sh` installs the sampler and enables its timer.
Install both by hand only on a host the provisioning script has not run against:

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

## The off-host copy of the record

The archive keeps every ANSWER for ever.
It keeps the RAW CROSSING LINES on this host for a window, and only for a window,
and it keeps them off the host for the run.
`coldcopy.sh` is what puts them off the host, and it is also the gate that lets
the archive remove one.

**No segment is removed until a receipt confirms a copy that matches the bytes on
this disk.** The gate is in the archive's own code. There is no timeout and no
override. So a cold archive that has stopped working costs disk and never
record.

### Turn it on

Set three values in `/etc/multiverse/deploy.env`:

```sh
MV_COLDCOPY=s3
MV_COLDCOPY_URI=s3://<bucket>/<prefix>
MV_COLDCOPY_STORAGE_CLASS=STANDARD
```

`MV_COLDCOPY_URI` names a live resource. Keep it on the host and in private
operations storage, and never in this repository.

Prefer a destination the instance can read and write **without a key on the
host**. A provider object store that is attached to the instance gives temporary
credentials through the instance metadata service, so no long-lived secret ever
lands on a public-facing box. If the destination needs a key instead, put the
key in its designated secret manager, record its name, owner and rotation date
in private operations storage, and set only `MV_COLDCOPY_PROFILE` here.

`STANDARD` is the shipped storage class and it is the one to keep unless the
store both prices classes separately and allows the class you choose.
An infrequent-access class looks cheaper and is denied outright by some buckets'
own policies; the symptom is that every upload returns 403, no receipt is
written, and nothing is ever retired.

**The AWS CLI is not an apt package on Ubuntu 24.04.** `apt-cache policy awscli`
returns an empty version table there with every component enabled — the package
is gone, and AWS ships v2 through its own installer only. `provision.sh --only
packages` therefore installs `unzip` and *detects* the CLI with `command -v aws`;
it does not try to install it and never claims to have. Install it once, from
AWS's own signed archive, and verify the signature rather than trusting the
download:

```sh
curl -fsSLO https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip
curl -fsSLO https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip.sig
gpg --verify awscli-exe-linux-x86_64.zip.sig awscli-exe-linux-x86_64.zip
unzip -q awscli-exe-linux-x86_64.zip && sudo ./aws/install
aws --version
```

`gpg --verify` must print `Good signature` from the published AWS CLI signing
key. That key's own expiry has lapsed and AWS has not rotated it, so gpg also
prints `[expired]`; a signature from the expected private key still establishes
origin, and expiry is a policy statement rather than a compromise indicator.
**Do not install the snap.** `multiverse-coldcopy.service` sets
`NoNewPrivileges=true`, a classic snap launches through the setuid
`snap-confine`, and `NoNewPrivileges` blocks it — the timer would then fail every
hour, which is the exact silent failure the cold-copy design exists to prevent.

Then, in order:

```sh
sudo deploy/provision.sh --only packages    # unzip, and reports the CLI it finds
sudo deploy/provision.sh --only systemd     # installs and enables the hourly timer
sudo -u multiverse /opt/multiverse/deploy/coldcopy.sh --check
sudo -u multiverse /opt/multiverse/deploy/coldcopy.sh --dry-run
sudo -u multiverse /opt/multiverse/deploy/coldcopy.sh --list
```

**`--check` before anything else.** The AWS CLI is not on this host by default,
and a destination that is unreachable, misnamed or denied fails in exactly the
same way as one that is absent: no receipt, so nothing retires, so the disk
fills while every other reading looks healthy. `--check` reads the destination
and uploads nothing. `provision.sh --only verify` runs it too.

`--check` cannot prove that a write will be accepted. Only the first real run
does that. Read its output.

### Read what is waiting

`ledgerSegmentsAwaitingColdCopy` on `/api/status` is the number that should be
zero. It counts closed segments that are past the window and have no confirmed
off-host copy.

```sh
curl -s http://127.0.0.1:8796/api/status | jq '{
  segments: .ledgerSegments,
  awaiting: .ledgerSegmentsAwaitingColdCopy,
  retired:  .ledgerRetiredTotal,
  rawFrom:  .ledgerRawWindowFromMs,
  windowMs: .ledgerWindowMs }'
```

`monitor.sh` watches it, and it is the only thing on this host that watches the
object store at all:

```sh
sudo -u multiverse /opt/multiverse/deploy/monitor.sh --only archive --quiet --verbose
```

| Reading | What it means |
|---|---|
| `0` | Every segment past the window has a confirmed copy. Retirement is working. |
| A small number, for less than an hour | A segment has just closed. The timer runs hourly. |
| The same number for a day | The copy is failing rather than lagging. Run `coldcopy.sh --check`. |
| Rising, with the raw window far past the configured one | The window is doing nothing. Nothing has been lost — the disk is what is at risk. |

`coldcopy.sh --list` prints the same picture from the host's own directory,
including retired segments, which appear as a receipt with no file beside it.
That receipt is the only record on this host of where those bytes went, and it
is deliberately kept after the segment is removed.

### Bring a segment back

```sh
sudo -u multiverse /opt/multiverse/deploy/coldcopy.sh --restore 2026-08-16-0000.jsonl.gz
```

It refuses to overwrite a segment that is already present.
It verifies the fetched object's `sha256` against the receipt and checks that
the bytes are a valid compressed stream, and it puts nothing into place until
both pass. A file that hashes correctly and will not decompress is still no
record.

The archive picks a restored segment up on its next start: segments are read in
order by the day inside the name, and a restored one simply reappears in the
run. It will be retired again once it is past the window and its receipt is
present, so keep the receipt only if you want it kept.

Restoring a segment does not repair a fold that has a hole in it. If
`/api/status` reports `replayFromRetired`, the archive is saying that its saved
cursor named a segment that had already left this host; restore the segment
first, then restart the archive so the fold is rebuilt across it.

## Daily billing reconciliation

`monitor.sh` watches the transfer allowance from this host's own NIC counter.
That counter needs no cloud credential, which is the only reason the check can run on a
public-facing host at all.
It is a proxy, and nothing on the host can audit it.

`ce-reconcile.sh` is the audit.
It runs on an operator machine, not on the host.
It reads Cost Explorer and the instance metric, compares them, and writes one small JSON file to
the host over `ssh`.
The host still holds no cloud credential.

```sh
export MV_AWS_PROFILE=<read-only profile>
export MV_LIGHTSAIL_INSTANCE=<instance name>
export MV_BILLING_HOST=<user>@<host>

deploy/ce-reconcile.sh --dry-run --print   # read, compute, ship nothing
deploy/ce-reconcile.sh --print             # read, compute, ship
```

The file lands at `/var/lib/multiverse/monitor/billing.json`, owned by the monitor's service
account and mode `0644`.
The write is atomic: `install` creates a temporary file with the final owner and mode, and one
`mv` replaces the old file.
The monitor reads it every five minutes and must never see half of it.

| Call | Count per run | Cost |
|---|---:|---|
| Cost Explorer `get-cost-and-usage` | 1 | `$0.01` |
| Lightsail `get-instance-metric-data` | 2 | free |

Daily is the correct cadence.
The provider's billing data refreshes only a few times a day and lags about 14 hours, so a more
frequent call costs more and reveals nothing.
The script makes exactly one Cost Explorer call and passes `--no-paginate`.
If the answer carries a `NextPageToken`, the script stops instead of paying for a second page.

Every figure uses the provider's GB of `2^30` bytes.
The instance metric sums about `1.01` times the Cost Explorer quantity over the same days.
That residual is systematic and is not corrected.
The ratio is published instead, and the monitor's `billing` check warns when it leaves its band.

The ratio compares the days Cost Explorer has settled, never month-to-date against month-to-date.
Cost Explorer lags, so the second comparison would show a difference that is only the lag.

A day is settled only after it has been over for `MV_BILLING_CE_LAG_HOURS`, which defaults to 18.
A day is not settled because it has data in it.
The newest day Cost Explorer returns always has data and always has less data than the day
contains, so counting it compares a whole day of counter against a part day of invoice.
The first real run of this tool reported `1.37` from two instruments that agree to `1.01`, for
exactly that reason.

At 06:30 UTC this makes the newest settled day the day before yesterday.
That is the honest reading. A fresher one would be a part day.

Transfer quantities come from settled days only.
A billed overage is reported from every day, settled or not, because it is money the provider has
already charged and not half of a comparison.

Run the reconciliation once a day from a machine that is reliably awake.
Record the exact schedule in private operations storage, because the schedule names a host.
If the reconciliation stops, the monitor reports it as a warning after `MV_BILLING_STALE_HOURS`.
It is a warning and not a critical: the allowance is still watched, and what was lost is the audit.

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

**Open item — a test identity is still holding a slot.** Installer testing follows the rule above
and enrols for real, so a discarded test install leaves a real identity on the map with no world
behind it. One is outstanding:

| Identity | World name | Slot | Created | What it needs |
|---|---|---|---|---|
| `public-2db3a641ac3a4b6491148ea67c496011` | `LauncherTest` | 10 | 2026-08-17, launcher and installer testing | Release the slot, then drop the credential |

Release it with the `release-slot` act on the relay's admin listener — report first, read the
consequence, then confirm within the token's ten minutes — and drop its credential from the
verifier store afterwards. The slot NUMBER is never reused, which is the point of a release rather
than an eviction: nothing that ever crossed to or from that world changes meaning. Add a row here
when a test enrols, and delete the row when it is released, so a stale identity is a line somebody
can read rather than a slot nobody can explain.

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

Its `systemd` phase is the exception, and it enables and starts both services.
Never run it while a reboot hold-down is in force: it undoes a `systemctl disable` and puts the
relay back in front of a replaying archive. `RESTART-POLICY.md`, "Host reboot", holds the order,
and the drop-in that phase installs is the hold-down that survives it.

**Install packages before you stage binaries, and never between staging and the restart.**
`needrestart` runs after every apt transaction and restarts services whose binaries have been
replaced on disk — the relay and the archive together, outside the peer gate and the relay
hold-down, which is the one restart shape that loses record permanently. `provision.sh` runs its
`needrestart` phase first and installs `/etc/needrestart/conf.d/90-multiverse.conf` so that no apt
transaction on the host can restart a multiverse unit, and it exports `NEEDRESTART_MODE=l` for its
own apt calls. Neither of those makes the ordering optional.
`RESTART-POLICY.md`, "Package installs and needrestart", carries the rule and the incident that
produced it.

Read `RESTART-POLICY.md` before a relay, archive, or host restart.
Batch archive changes: a restart costs a held-down relay, and that outage is a
participant outage even though it is now bounded by the duplicate window rather
than by the length of the run. Read the projected cost from
`monitor.sh --only replay`'s `replay-cost` verdict before you announce one.

An archive change that alters what the archive retains per record is not finished until
`MV_REPLAY_RESIDENT_B`, `MV_REPLAY_PEAK_B` and `MV_ARCHIVE_GOMEMLIMIT` are re-derived in
`deploy.env` from a measurement of the new build. Those constants describe a binary, not
the project. `SIZING.md`, "Archive memory", carries the current values and the rule.
Apply them with `provision.sh --only envfiles`; the archive reads them at start.

Use `test-front-door.sh` after an nginx template or announcements-page change.
Use `test-units.sh` after a systemd unit change.
Use `test-monitor.sh` after a `monitor.sh` change.
Use `test-coldcopy.sh` after a `coldcopy.sh` change.
Use `test-viewers-presence.sh` after a `viewers-presence.sh` change.
Use `test-ce-reconcile.sh` after a `ce-reconcile.sh` change.
Use `test-ci-gate.sh` after a `ci-gate.sh` change.
Use `test-deploy.sh` after a `deploy.sh` change.
Use the provisioning verification phase after any host change.

Use `health-snapshot.sh --watch` across the change window.
The verification phase reports that each check passed, and a service can be measurably worse with
every check still passing.

### What a deployment leaves on the host

Each deployment stages a kit copy, replaces binaries, and can leave a copy of a file it edited.
These copies are rollback references. They are useful for one generation and clutter after that.
This is host housekeeping. It is not `MV_RETENTION`, which governs participant data.

Keep on the host:

- The installed kit in `/opt/multiverse/deploy/`, and the staging copy it was installed from.
- The previous kit staging copy.
- The installed binaries in `/opt/multiverse/bin/`, and the previous binaries beside them.
- The staging directory that `ship.sh` writes, `MV_STAGE_DIR`. Each `ship.sh` run overwrites the
  artifacts and their `SHA256SUMS`, so it never accumulates. **An artifact put there by hand is a
  different matter**: `provision.sh --only binaries` reads whatever sits under the canonical
  `<cmd>-linux-<arch>` names, and a leftover from an earlier deployment under those names is
  indistinguishable from a fresh one. It happened on 2026-08-17 — the phase reported `already
  current` for all three binaries and installed nothing, truthfully, about the wrong files. The
  phase now verifies `SHA256SUMS` before it compares anything, prints the staged and installed
  checksum for each artifact, and says loudly when it installed nothing. Pin what a run must
  install and it refuses instead:

  ```sh
  sudo provision.sh --only binaries \
    --expect-sha256 archive=<sha256> --expect-sha256 relay=<sha256>
  ```
- The last two `.bak-*` copies of any file under `/etc/multiverse/`. Nothing in this kit writes
  those; an operator makes them by hand, and only the operator removes them.

Remove after the next deployment succeeds:

- Every kit staging copy older than the previous one.
- Every binary copy older than the previous one.
- Every `.bak-*` copy beyond the last two.

Remove nothing while a deployment record is still open. A record that is open still names its
rollback artifacts.

A deploy script belongs in this repository, not in a home directory. A script under `/home/<user>`
is unversioned, unreviewed, and gone when the instance is replaced. If a deployment needs a step
this kit does not have, add the step here and ship it. The kit copy under `/home/<user>` that
"Install a host" creates is not a deploy script; it is the payload that `provision.sh` installs.

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
deploy/test-viewers-presence.sh
deploy/test-ce-reconcile.sh
deploy/test-ci-gate.sh
deploy/test-deploy.sh
```

Both restart procedures answer `--dry-run`, which walks every step and changes nothing:

```sh
deploy/restart-relay.sh --dry-run
deploy/restart-archive.sh --dry-run
```

A dry run leaves no scratch directory behind, and neither does a real run, a refusal or an
interrupt. `restart-archive.sh` copies `deploy.env` into `${TMPDIR:-/tmp}/multiverse-restart-*` to
read the replay verdict, and `restart-lib.sh` removes it from the same EXIT trap that takes the peer
gate down. A leftover directory of that name means an old kit is installed; each one holds a copy of
every deployed parameter.

`test-monitor.sh` needs no root, no network and no host.
It runs `monitor.sh --only transfer`, `--only hosts-pin`, `--only replay`, `--only swap` and
`--only billing` against fake `/proc` files, a fake `/etc/hosts`, a fake status document, a fake
reconciliation file and a fixed clock, in a temporary state directory.
Its replay cases hold the archive memory gate to physical RAM: the same reading with a swap file
present must keep the same ratio and the same severity.
Its billing cases hold the check to reporting absence: a reconciliation that never arrived and one
that stopped arriving must both be readings, not silence.
Its exit-status cases hold the code to one meaning: `OK`, `WARN` and `CRIT` all exit `0`, and only
a pass that could not start exits `2`.

`test-ce-reconcile.sh` needs no AWS account.
It runs `ce-reconcile.sh --from-file` against a saved response in `deploy/testdata/` and a fake
instance-metric provider, so it makes no Cost Explorer call and spends nothing.
A Cost Explorer call costs `$0.01`, and its answer changes daily, so the live API is the wrong
place to test a parser.

`test-viewers-presence.sh` needs no root, no network and no host either.
It runs `viewers-presence.sh` against a written access log, a written metrics file and a fixed
clock.
Its cases are the four ways the signal has to be right: silence reads as nobody, a failed request
still proves that a person is waiting, a loopback probe is not an audience, and a player that
reached the stream counts whatever the log says.

Run `shellcheck` when it is available.
Run `systemd-analyze verify` against the units on a compatible Linux host.
Run the repository link and release checks before publication.
