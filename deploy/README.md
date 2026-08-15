# `deploy/` — the hosting kit

**What this is.** Everything the hosted Bibites Multiverse service needs.
The service went live on AWS Lightsail on 2026-08-14.

**What it is for.** `m5_considerations.md`'s Design Question 2 says the hosted
relay is a service and not a process, and names six operational obligations.
Every one of them has a file here:

| DQ2's obligation | Where it lives |
|---|---|
| A supervisor — restart on exit, start on boot | `systemd/multiverse-{relay,archive,stream}.service` |
| Monitoring that speaks when nobody is looking | `monitor.sh` + `systemd/multiverse-monitor.{service,timer}` |
| Backup of the irreplaceable files | `backup.sh` + `systemd/multiverse-backup.{service,timer}` |
| A written restart policy | `RESTART-POLICY.md` |
| A name, a certificate and its renewal | `tls-deploy-hook.sh`, `provision.sh --only tls`, `nginx/` |
| D24's announced period and its ending | `WIND-DOWN.md`, `ANNOUNCEMENT.md` |

Plus the two things WP3 owes beyond DQ2: Decision 3's retention rule with **the
arithmetic a hoster sizes a volume from** (`SIZING.md`), and the deliberate
decision about publishing the status page (`nginx/multiverse-20-status.conf`).

---

## 1. The files

| File | What it is |
|---|---|
| `README.md` | This index, the execution order, and the owner's manual steps |
| `deploy.env.example` | **The one parameter file.** Copy it and fill the owner-specific values |
| `provision.sh` | Idempotent provisioning of a fresh Lightsail Ubuntu instance, in 16 named phases. `--dry-run`, `--only <phase>` |
| `install-stream-origin.sh` | Installs verified MediaMTX, private RTMP ingest, and loopback HLS |
| `ship.sh` | Runs on the **development** machine: cross-compiles both architectures and scps them over. The only script here that does not run on the instance |
| `issue-join.sh` | Mints a participant's join string. Takes the relay restart issuance requires, deliberately and in a batch |
| `monitor.sh` | Fourteen checks, five minutes apart, alerting a person on change. `--test` proves the channel |
| `backup.sh` | Three tiers: identity hourly, the record daily, and the offsite half documented. `--restore-help` prints the procedure |
| `tls-deploy-hook.sh` | certbot deploy hook: installs the renewed pair and reloads nginx. No service restart |
| `test-front-door.sh` | Renders the nginx templates and checks the shared HTTPS front door |
| `systemd/*.service`, `systemd/*.timer` | Seven units: three services, two timers, and their oneshots |
| `nginx/multiverse-10-acme.conf` | Port 80: the ACME challenge and nothing else |
| `nginx/multiverse-20-status.conf` | HTTPS 443: the website, `/stream/` HLS, and `/contract-b/` WebSocket proxy |
| `SIZING.md` | Decision 3's arithmetic: the growth rule, the memory model, the tripwire and the sizing procedure |
| `RESTART-POLICY.md` | WP3's written restart policy, built from the measured replay arithmetic |
| `WIND-DOWN.md` | D24's ending as a stated event: the timeline, both retention arms, extension, and the publish-the-relay path |
| `ANNOUNCEMENT.md` | The text for `@@OWNER:ANNOUNCED_PERIOD@@` and the five documentation slots other packages left for WP3. **Edits nothing** |

## 2. What is parameterized

The live values are in `/etc/multiverse/deploy.env`. The file is not stored in
this repository because it contains the alert capability URL.

| Waiting on | Variables | What it changes |
|---|---|---|
| **The domain** | `MV_DOMAIN`, `MV_CERT_EXTRA_NAMES`, `MV_ACME_EMAIL` | The certificate, advertised relay URL, nginx name, and loopback pin. The live domain is `bibitesmultiverse.com` |
| **The alert channel** | `MV_ALERT_KIND`, `MV_ALERT_URL`, `MV_ALERT_COMMAND` | Where `monitor.sh` sends. The live alert channel has passed its test |

**The memory verdict is settled and filled in.** `MV_ARCHIVE_GOMEMLIMIT=5GiB`,
`MV_ARCHIVE_MEMORY_HIGH` empty, `MV_SWAP_GB=0` — because the replay was fixed
rather than tuned: it streams the ledger instead of materialising it, at 184 B of
peak per record against 1,030–1,286 B. `SIZING.md` §4 has the numbers and
`deploy.env.example` has the reasoning beside each value.

**The retention rule is settled and filled in.** Decision 3, answered by the
owner on 2026-08-12: `MV_RETENTION=prune-genomes` with
`MV_ARCHIVE_GENOME_HORIZON=720h`. **The ledger is kept forever** and genome blobs
are pruned to a thirty-day horizon, which makes Risk 7 a stated policy instead of
an accident and is announced before anybody joins. It buys disk and egress, not
RAM: the resident set still grows with ledger records. The live instance uses
the `small_3_0` bundle described in `SIZING.md` §4.

The public front door is `MV_RELAY_PORT=443`. The private relay upstream is
`MV_RELAY_BACKEND=127.0.0.1:8795`. The archive stays at `127.0.0.1:8796`.
The HLS origin stays at `127.0.0.1:8888`.

## 3. Execution order, the day the instance exists

**Prerequisites: §6 steps 1–10.** An instance exists, it has a static IP, the
domain's A record points at it, and the Lightsail console firewall is open. The
sequence below is what happens after that, and every command is literal.

```sh
# ---- on the DEVELOPMENT machine -------------------------------------------
# 1. Build both architectures and send them. No Go toolchain goes on the VPS.
cd /mnt/wsl/data/repos/bibites-multiverse
deploy/ship.sh --host <instance-ip>          # or an ~/.ssh/config alias

# 2. Send the kit itself.
scp -r deploy ubuntu@<instance-ip>:/home/ubuntu/multiverse-kit

# ---- on the INSTANCE -------------------------------------------------------
ssh ubuntu@<instance-ip>

# 3. The parameter file. Confirm the domain, bundle, and period. Fill in
#    MV_ACME_EMAIL and the selected alert fields.
#
#    THE GROUP IS LOAD-BEARING. `install` under sudo produces root:root unless
#    -o/-g say otherwise, and monitor.sh and backup.sh run as the `multiverse`
#    user and exit 2 on an env file they cannot read — so root:root here means
#    the monitoring timer and the backup timer fail on every tick with nothing
#    announcing it. `install -g` needs the group to exist, and provision.sh's
#    `account` phase is what normally creates it, so create it here first: the
#    line below is idempotent and that phase will simply say "already".
getent group multiverse >/dev/null || sudo groupadd --system multiverse
sudo install -d -m 0750 -o root -g multiverse /etc/multiverse
sudo install -o root -g multiverse -m 0640 \
     /home/ubuntu/multiverse-kit/deploy.env.example /etc/multiverse/deploy.env
sudo nano /etc/multiverse/deploy.env
ls -l /etc/multiverse/deploy.env      # -rw-r----- 1 root multiverse

# 4. Rehearse. It changes nothing, and it checks the A record, the staged
#    binaries and every value above.
sudo /home/ubuntu/multiverse-kit/provision.sh --dry-run

# 5. Provision. Fifteen phases, idempotent, a few minutes.
sudo /home/ubuntu/multiverse-kit/provision.sh

# 6. Prove the alert channel BEFORE trusting it.
sudo -u multiverse /opt/multiverse/deploy/monitor.sh --test
sudo -u multiverse /opt/multiverse/deploy/monitor.sh --verbose

# 7. Prove the backup, and read the restore procedure once while nothing is
#    wrong.
sudo systemctl start multiverse-backup.service
sudo -u multiverse /opt/multiverse/deploy/backup.sh --list
/opt/multiverse/deploy/backup.sh --restore-help | less

# 8. The owner's own join string, and the map's first slot.
sudo /opt/multiverse/deploy/issue-join.sh peer-<name>
#    Then raise MV_EXPECTED_PEERS to the count the map should hold. It ships at
#    0, which means NO FLOOR — the peers check watches dark slots only, because
#    a floor of 1 pages on the first tick of a map nobody has joined yet. Until
#    you raise it, "peers lost" is unwatched and monitor.sh says so on every run.

# 9. Fill the documentation slots from ANNOUNCEMENT.md, publish the release,
#    and only then hand a join string to anybody else.
```

**Step 9 is a gate, not a formality.** D24's period must be stated *before*
anybody joins, and `release/README.md`'s first publish step is already waiting on
that text.

Re-running `provision.sh` is the normal way to apply a changed `deploy.env`, a
new binary or a kit update. It never restarts the two services — a new binary on
disk is not a running new binary, and the restart is a deliberate act with
`RESTART-POLICY.md` behind it.

## 4. The TLS design and shared HTTPS front door

**nginx terminates TLS on port 443.** It sends `/contract-b/` WebSocket requests
to the relay on `127.0.0.1:8795`. It sends `/stream/` to MediaMTX on
`127.0.0.1:8888`. It sends all other HTTPS paths to the archive on
`127.0.0.1:8796`.

nginx replaces `X-Forwarded-For` with the direct client address. The relay
trusts one forwarded address only when the direct peer is on loopback. This rule
preserves the per-address connection limit and rejects client-supplied chains.

**Issuance and renewal: certbot, HTTP-01, webroot, through nginx on port 80.**

- **Why not standalone on 80.** certbot's standalone plugin binds 80 itself,
  which means stopping whatever is there. nginx is on the box anyway for the
  status page, so a webroot costs nothing extra and never contends for a port.
- **Why not TLS-ALPN-01 on 443.** The webroot flow is already deployed and
  verified. It also keeps certificate issuance separate from application paths.
- **Why not DNS-01.** It is the better answer and it is not available yet: it
  needs a certbot plugin for a registrar the owner has not chosen. It is wired as
  `MV_ACME_MODE=dns` and `provision.sh` **refuses rather than guessing a
  provider**. Switch to it once the registrar is known and port 80 can close.
- **So port 80 is open.** It is the third public rule beside ports 22 and 443.
  It serves short-lived challenge files. `/` redirects to the HTTPS website.

**The deploy hook keeps one served copy.** It writes the certificate for the
monitor and keeps the private key readable only by root. It then reloads nginx.
The monitor compares the served certificate with the installed certificate.

**A rotation costs no restart.** An nginx reload keeps existing WebSocket and
website connections open while new workers use the renewed pair.

## 5. The website and live console are published deliberately

`docs/defaults-audit.md` routed a finding to WP3: the project's own bring-up
passes `ARCHIVE_HTTP=0.0.0.0:8796` and the runbook says so in five places. On a
LAN with six known worlds that is right. On a public VPS it is *"a hoster copying
a LAN habit onto the public internet"* — an unauthenticated, unrated, unproxied
listener on a box whose disk the same process is filling.

**The decision: the website and console are public, and not like that.**

- **Public, because WP7's participant documentation promises it.** The root
  page explains the project and shows a live summary. The `/live` console shows
  the complete map. `diagnose.md` tells participants to read the console.
  `error-taxonomy.md` §3.2 tells a refused peer to read the current game build
  there. A page nobody can reach makes both documents wrong.
- **Not by binding the archive to `0.0.0.0`.** `MV_ARCHIVE_HTTP` stays
  `127.0.0.1:8796`, which is the compiled default and the right one. nginx
  publishes the website at `https://bibitesmultiverse.com/` and the console at
  `https://bibitesmultiverse.com/live`, **GET and HEAD only**. The surface is
  rate limited to 5 r/s with a burst of 20 and 8 concurrent connections
  per address. nginx does not compress the responses, so the archive controls
  gzip negotiation. The front door also sets HSTS, CSP, Permissions Policy,
  and same-origin opener headers.
- **The boundary, named — as a rule, because the list grows.** Public: **every
  handler on the archive's HTTP mux, and the mux is read-only by construction.**
  nginx proxies `location /` wholesale, so a handler added to
  `go/internal/archive/page.go` is published the moment the binary ships; the
  rule, not the roster, is what a reviewer checks. At the commit this kit was
  last read against, that is **seventeen** read-only handlers — `/`, `/live`, `/watch`,
  `/map`, `/favicon.svg`, `/social-card.svg`, `/robots.txt`, `/sitemap.xml`,
  `/healthz`, `/api/status`, `/api/hops`, `/api/species`, `/api/species/tree`,
  `/api/species/trends`, `/api/species/brains`, `/api/species/history`,
  `/api/history`. **The two questions a new endpoint has to answer** are whether
  it mutates anything (none of these does, and `limit_except GET HEAD` is the
  second half of that answer) and whether it carries anything the relay does not
  already broadcast. Nothing here is a secret, because nothing on this wire is:
  what the page shows is what the relay already broadcasts to every peer, and
  `join.md` states that in full before anybody joins. Not public: the archive's
  own listener, the relay's admin path, the data directories, the verifier store,
  SSH.
- **The stream has a separate write boundary.** Viewers can read `/stream/`.
  The RTMP publisher uses `172.26.12.110:1935` through private VPC peering.
  The host firewall permits only `172.31.0.0/16`. MediaMTX also requires a
  random publish password. See `docs/live-broadcast.md`.
- **The cost, named.** The stream is the largest egress term at approximately
  810 GB per continuously open viewer-month. The live console uses approximately
  32 GB uncompressed or 4 GB gzipped. Add a video CDN before the stream exceeds
  the Lightsail transfer allowance.

**B28's admin path stays off.** `MULTIVERSE_RELAY_ADMIN_LISTEN` is deliberately
absent from the generated environment file; the listener is compiled in and bound
to nothing until an owner-level act turns it on, and off loopback it also demands
TLS. `provision.sh --only verify` checks that it is still unset.

## 6. Cloud deployment record and remaining work

The instance, static address, DNS, certificate, alarms, snapshots, and services
are live. The remaining release work is listed in step 12.

1. **Create an AWS account** (or sign in to an existing one). `console.aws.amazon.com`.
2. **Check Lightsail free-trial eligibility.** New accounts get **90 days free**
   on the $5/$7/$12 Linux plans, **one bundle per account**, for accounts that
   started using Lightsail on or after 2021-07-08. D24's period is three months
   and the $12 bundle is on the list, so **if the retention rule bounds the
   ledger the announced run can cost $0 in compute** — worth exploiting and worth
   not depending on. Verify before announcing anything. *(Prices and terms from
   `wp3_hosting_options.md`, fetched 2026-08-11; re-check at purchase.)*
3. **Nothing — Decision 3 is answered.** `MV_RETENTION=prune-genomes`,
   `MV_ARCHIVE_GENOME_HORIZON=720h`: the ledger is kept forever and genome blobs
   are pruned to thirty days. The replay experiment that used to gate this step
   reported on 2026-08-12 and removed the memory constraint rather than answering
   the question, so the rule was decided on what participants are promised.
   **What is left here is the bundle**, and pruning blobs does not shrink the
   resident set — read step 4 against `SIZING.md` §4, not against the disk table.
4. **Create the instance.** Lightsail → Create instance → **Linux/Unix** →
   **OS Only → Ubuntu 24.04 LTS** → region **US East**. Bundle per step 3:
   **$12 (2 GB / 60 GB / 3 TB)** if the rule bounds the ledger; **$44 (8 GB /
   160 GB / 5 TB)** for a ledger that grows all run — which on the streaming
   archive holds the archive resident through day 110 and can still **restart**
   it at day 180, so it carries the announced period whole. Lightsail resizes
   through a snapshot, so starting small and moving up on `monitor.sh`'s replay
   tripwire is a legitimate plan that converts a sizing error into a bill.
   `SIZING.md` §5.
5. **Attach a static IP.** Lightsail → Networking → Create static IP → attach.
   **Do this before the A record**: a Lightsail instance's default public address
   changes when it is stopped and started.
6. **Register the domain and point it.** The registrar comparison and the choice
   — Cloudflare Registrar, Porkbun as the fallback — are in
   `wp3_hosting_options.md`, *Which registrar to buy from, and which name to
   buy*, and the purchase has its own handoff at `HANDOFF-domain.md`. Create an **A record**
   for the name at the static IP, TTL 300 while setting up. If an **AAAA** record
   exists it must point at the same instance, or the ACME validator will reach
   the wrong host over IPv6. **The name is permanent for the run.**
7. **Open three ports in the Lightsail console's own firewall** — Networking →
   IPv4 Firewall: **22** (SSH), **80** (ACME), and **443** (HTTPS). ufw is the
   second of two firewalls and the console's is the one
   the internet meets first. Repeat under IPv6 Firewall if the instance is
   dual-stack.
8. **Create the alert topic.** One line, no account:
   `echo "https://ntfy.sh/multiverse-$(head -c 9 /dev/urandom | base64 | tr -d '/+=')"`.
   Subscribe on the phone app or in a browser tab. Put it in
   `MV_ALERT_URL`. Treat it as a secret: whoever knows the URL can read and post.
9. **Create a Lightsail metric alarm** — the instance's **status check failed** →
   notify by email. This is the only watcher that survives the box being dead;
   everything in `monitor.sh` runs *on* the box.
10. **Enable automatic snapshots** — Lightsail → the instance → Snapshots →
    Enable, pick an hour. This is the offsite half of `backup.sh` and the only
    thing that genuinely protects the ledger and the genome store. Roughly
    $0.05/GB-month, incremental after the first *(carried knowledge — check the
    pricing page)*.
11. **Hand over SSH access, or run §3 steps 1–9 yourself.** Either works; the scripts
    assume the Lightsail default user `ubuntu` and a key the owner holds.
12. **Publish**: fill the slots from `ANNOUNCEMENT.md`, complete
    `release/README.md`'s four steps, and only then issue the first join string
    to anybody but yourself.

## 7. What this kit cannot cover

Stated rather than left to be discovered.

- **External reachability must be tested outside the instance.** `/etc/hosts` pins
  the domain to loopback so the archive can dial the relay by name over a
  certificate that names only the domain — on Lightsail the public address is
  NAT'd and is not reachable from the instance itself. Every check therefore
  takes the inside path. **The first proof that a stranger can reach the map is a
  outside client reaching the map**. The live website and relay path passed this
  check after deployment.
- **The forward receipt is not here.** B26 ships from the parallel arc.
  `RESTART-POLICY.md` §1.2 describes the world *without* it and says so; the
  paragraph is marked and should be revised when B26 deploys.
- **The measured per-migration cost of the receipt at rate** is WP3's done-when
  and belongs to that arc plus WP8, not to this one.
- **A dry run remains the first deployment step.** The live instance also runs
  `nginx -t` before each reload and the full verification phase after changes.
- **`monitor.sh` cannot alert when the instance is off.** That is what step 9's
  Lightsail alarm and the daily heartbeat are for: the heartbeat's *absence* is
  the signal.
- **The retention rule itself is a decision, not a script.** The kit makes it a
  variable, ships the arithmetic and writes both endings. Choosing is the owner's,
  and Decision 3 requires it before D24's announced ending.

## 8. Verification performed

Static checks:

| Check | Result |
|---|---|
| `bash -n` on all eight scripts | clean |
| `systemd-analyze verify` on all seven units | clean on systemd 255 — the only notices are "command not found" for paths that exist on the instance and not here |
| `test-front-door.sh` | clean render; it also runs `nginx -t` when nginx is available |
| `nginx -t` | clean on the live Ubuntu 24.04 instance before reload |
| `shellcheck` | **not run** — not installed here. The scripts are written to its rules: `set -uo pipefail` (`-e` only where a partial run would be worse than a stop), every expansion quoted, `read -r`, `$(...)`, no unquoted globs, arrays for argument lists, and one annotated `disable=SC2086` where the operator's value carries its own arguments |

Behavioural checks, against the real binaries built by `ship.sh` from this
commit, on scratch ports and a scratch data directory. **The living deployment
was not touched** and every build and process ran under `nice -n 19`, per the
rig's own constraint:

| What was proven | How |
|---|---|
| **The cross-compile is real.** `linux/amd64` and `linux/arm64`, both **statically linked** | `ship.sh --build-only`, then `file` |
| **The generated environment files drive both binaries with no arguments at all** | Both started from `relay.env` / `archive.env` alone; `ExecStart` carries nothing |
| **nginx serves the website and proxies `/contract-b/` on HTTPS 443** | off-box HTTPS and WebSocket-path checks |
| **nginx serves one HLS stream below `/stream/`** | authenticated synthetic RTMP publish, then a public LL-HLS manifest with H.264/AAC |
| **Both application listeners stay on loopback** | `ss`, provisioning verification, and an off-box port check |
| **The bootstrap mint is what lets the relay start at all.** An empty credential store makes it refuse to serve and exit 1 — which is why `bootstrap` precedes `systemd` | Observed, then re-run in the correct order |
| **The archive subscribes with the bootstrap-minted `subscribe` credential** read from `MULTIVERSE_CREDENTIAL_FILE` | `relayConnected: true`, `haveStatus: true` |
| **Certificate renewal needs no service restart** | certbot deploy hook installs the pair and reloads nginx |
| **`monitor.sh`'s fourteen checks run, escalate and recover** — including the stream-origin listener and lane-bypass persistence | A synthetic `/api/status` server, the live HLS origin, and a synthetic state directory |
| ↳ **The replay check's constants changed on 2026-08-12 and have NOT been re-rehearsed.** It now projects both terms and alerts on the larger, so the same 7.43 M records project ~2.1 GB (resident) rather than ~9.4 GB. The escalate-and-recover behaviour around it is untouched | — |
| **`monitor.sh`'s error counter survives the numbered log rotation** | Appended errors, rotated `relay.log` → `relay.log.1`, confirmed no double count and no missed lines |
| **`backup.sh` produces, prunes and round-trips** — identity snapshots with checksums, `MV_BACKUP_KEEP` pruning, gzip of the ledger that `gunzip | diff` matches, the hardlink genome snapshot, and the `auto` guard correctly refusing to copy a ledger onto a filesystem below its free-space floor | Synthetic data directory |
| **The `deploy.env` ownership is asserted, not assumed.** `envfiles` stops with a named remedy when the group does not exist yet, and otherwise chowns and chmods the file; `verify`'s new check reads the file *as* the service user and prints the `chown` remedy when it cannot | A scratch env file and an existing group, `--dry-run` for the mutation and a real `runuser` for the check |
| **`monitor.sh` and `backup.sh` distinguish a missing env file from an unreadable one**, each exiting 2 with the cause rather than the symptom | A path that does not exist, and a file at mode 000 |
| **The A-record preflight tells a `/etc/hosts` pin from a wrong record.** A `127.` answer prints as the pin it is with the off-box `dig` to run instead; a different address still warns exactly as before | `getent` against a loopback-pinned name and against a public one, with the instance-metadata answer stubbed |
| **`monitor.sh`'s peers check at every floor** — 0 with an empty map (OK, and it says the floor is unset), 0 with a dark slot (WARN), a floor above the live count (CRIT), and a floor met (OK) | A synthetic `/api/status` server |
| **`provision.sh` catches the announced period's placeholder** — `YYYY-MM-DD`, a shaped non-date, and an end before its start all warn; a real pair prints the period in days | `--only preflight` against four scratch env files |

One observation worth recording rather than filing as a defect: on a six-slot
fixture the status frame compresses ~10×, but on the empty test map the frame was
573 bytes and came back uncompressed — correctly, because the archive leaves
anything under `gzipMinBytes = 1400` alone. Compression is a real term at real
map sizes and a no-op at trivial ones.

The live deployment record is in `HANDOFF-lightsail.md` and `dev_environment.md`.
