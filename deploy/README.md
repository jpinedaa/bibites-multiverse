# `deploy/` — the hosting kit

**What this is.** Everything the hosted Bibites Multiverse service needs, built
so that it can be reviewed today and executed the day an instance exists. It is
scripts and documents. **Nothing here has been run against a cloud account, no
account exists, nothing is signed up for and nothing is deployed.**

**What it is for.** `m5_considerations.md`'s Design Question 2 says the hosted
relay is a service and not a process, and names six operational obligations.
Every one of them has a file here:

| DQ2's obligation | Where it lives |
|---|---|
| A supervisor — restart on exit, start on boot | `systemd/multiverse-{relay,archive}.service` |
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
| `deploy.env.example` | **The one parameter file.** Copy to `/etc/multiverse/deploy.env` and fill in four values |
| `provision.sh` | Idempotent provisioning of a fresh Lightsail Ubuntu instance, in 15 named phases. `--dry-run`, `--only <phase>` |
| `ship.sh` | Runs on the **development** machine: cross-compiles both architectures and scps them over. The only script here that does not run on the instance |
| `issue-join.sh` | Mints a participant's join string. Takes the relay restart issuance requires, deliberately and in a batch |
| `monitor.sh` | Thirteen checks, five minutes apart, alerting a person on change. `--test` proves the channel |
| `backup.sh` | Three tiers: identity hourly, the record daily, and the offsite half documented. `--restore-help` prints the procedure |
| `tls-deploy-hook.sh` | certbot deploy hook: installs the renewed pair where the relay's `CertReloader` is already watching. No relay restart |
| `systemd/*.service`, `systemd/*.timer` | Six units: two services, two timers and their oneshots |
| `nginx/multiverse-10-acme.conf` | Port 80: the ACME challenge and nothing else |
| `nginx/multiverse-20-status.conf` | The status page, published deliberately — TLS, read-only, rate limited, on its own port |
| `SIZING.md` | Decision 3's arithmetic: the growth rule, the memory model, the tripwire and the sizing procedure |
| `RESTART-POLICY.md` | WP3's written restart policy, built from the measured replay arithmetic |
| `WIND-DOWN.md` | D24's ending as a stated event: the timeline, both retention arms, extension, and the publish-the-relay path |
| `ANNOUNCEMENT.md` | The text for `@@OWNER:ANNOUNCED_PERIOD@@` and the five documentation slots other packages left for WP3. **Edits nothing** |

## 2. What is parameterized, and on what

Four things were not settled when the kit was built; **two of them closed on
2026-08-12 and two are left.** Every one of them is a variable in `deploy.env`
and **none of them changes a command**.

| Waiting on | Variables | What it changes |
|---|---|---|
| **The domain** — the owner is registering a name | `MV_DOMAIN`, `MV_CERT_EXTRA_NAMES`, `MV_ACME_EMAIL` | The certificate, the advertised URL in every join string, nginx's `server_name`, the loopback pin in `/etc/hosts`. **Permanent for the run**: the URL is baked into every join string and D25's channel pushes nothing, so changing it later is a message to people who may be unreachable |
| **The alert channel** | `MV_ALERT_KIND`, `MV_ALERT_URL`, `MV_ALERT_COMMAND` | Where `monitor.sh` sends. Default `ntfy`, which needs no account |

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
RAM: the resident set still grows with ledger records, so the box is still sized
from `SIZING.md` §4. `MV_BUNDLE` stays open because nothing has been bought.

Two more that are decided and still variables, because a port is not a constant:
`MV_RELAY_PORT=443` (the owner's call) and `MV_STATUS_PORT=8443`.

## 3. Execution order, the day the instance exists

**Prerequisites: §6 steps 1–10.** An instance exists, it has a static IP, the
domain's A record points at it, and the Lightsail console firewall is open. The
sequence below is what happens after that, and every command is literal.

```sh
# ---- on the DEVELOPMENT machine -------------------------------------------
# 1. Build both architectures and send them. No Go toolchain goes on the VPS.
cd /mnt/wsl/data/bibites-multiverse
deploy/ship.sh --host <instance-ip>          # or an ~/.ssh/config alias

# 2. Send the kit itself.
scp -r deploy ubuntu@<instance-ip>:/home/ubuntu/multiverse-kit

# ---- on the INSTANCE -------------------------------------------------------
ssh ubuntu@<instance-ip>

# 3. The parameter file. Fill in MV_DOMAIN, MV_ACME_EMAIL, MV_ALERT_URL,
#    MV_BUNDLE and the two period dates. The retention rule and the memory
#    verdict already carry their answers.
sudo install -d -m 0750 /etc/multiverse
sudo install -m 0640 /home/ubuntu/multiverse-kit/deploy.env.example \
                     /etc/multiverse/deploy.env
sudo nano /etc/multiverse/deploy.env

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

## 4. The TLS design, and why port 80 is the fourth firewall rule

**The relay terminates its own TLS on 443.** That is the owner's call and it is
also the cheap one: B23 already put TLS at the relay's front door with a
rotation-surviving reload, so fronting it with a proxy would either duplicate or
throw away work that is done and tested. A certificate binds **names, not
ports**, so one certificate covers both the relay's `wss://` on 443 and the
status page on its own port.

**Issuance and renewal: certbot, HTTP-01, webroot, through nginx on port 80.**

- **Why not standalone on 80.** certbot's standalone plugin binds 80 itself,
  which means stopping whatever is there. nginx is on the box anyway for the
  status page, so a webroot costs nothing extra and never contends for a port.
- **Why not TLS-ALPN-01 on 443.** It needs the thing holding 443 to speak the
  ACME ALPN protocol. The relay does not, and teaching it to would put an ACME
  client inside the process D1 keeps deliberately dumb.
- **Why not DNS-01.** It is the better answer and it is not available yet: it
  needs a certbot plugin for a registrar the owner has not chosen. It is wired as
  `MV_ACME_MODE=dns` and `provision.sh` **refuses rather than guessing a
  provider**. Switch to it once the registrar is known and port 80 can close.
- **So port 80 is open**, and it is the fourth rule beside 22, 443 and the status
  port. What it serves is one directory of short-lived challenge files; `/`
  redirects to the status page and everything else is a 404.

**The permissions story: a copy hook, not a group grant.** The relay must read
the private key without running as root. The hook copies the renewed pair into
`/etc/multiverse/tls/` as `0640 root:multiverse`, atomically, and the relay runs
as a member of that group. The alternative — an ACL or group bit on
`/etc/letsencrypt` — has to hold across `live/`, `archive/`, a dated
subdirectory and the key itself, four objects certbot recreates on its own
schedule and whose modes it has reset across versions. The copy is one directory,
one owner, one mode, and `ls -l` is the whole story. Its own failure mode — a
hook that silently stops running — is what `monitor.sh`'s **cert** check watches,
by comparing the certificate the listener *serves* against the file on disk.

**A rotation costs no restart.** `GetCertificate` is called once per handshake and
stats the pair, so a renewed pair is picked up by the next handshake with no
signal and no dropped session. This was verified in WP2. nginx is the only
process here that needs telling, and the hook reloads it.

## 5. The status page is published deliberately

`docs/defaults-audit.md` routed a finding to WP3: the project's own bring-up
passes `ARCHIVE_HTTP=0.0.0.0:8796` and the runbook says so in five places. On a
LAN with six known worlds that is right. On a public VPS it is *"a hoster copying
a LAN habit onto the public internet"* — an unauthenticated, unrated, unproxied
listener on a box whose disk the same process is filling.

**The decision: the page is public, and not like that.**

- **Public, because WP7's participant documentation promises it.**
  `diagnose.md` tells participants to read it, and `error-taxonomy.md` §3.2 makes
  it the place a refused peer reads the game build the map is on rather than
  asking the operator. A page nobody can reach makes both of those documents
  wrong.
- **Not by binding the archive to `0.0.0.0`.** `MV_ARCHIVE_HTTP` stays
  `127.0.0.1:8796`, which is the compiled default and the right one. nginx
  publishes it on `MV_STATUS_PORT` over the same certificate, **GET and HEAD
  only**, rate limited to 5 r/s with a burst of 20 and 8 concurrent connections
  per address, with no compression of its own so the archive's gzip negotiation
  survives intact.
- **The boundary, named.** Public: the seven read-only handlers — `/`,
  `/healthz`, `/api/status`, `/api/hops`, `/api/species`, `/api/species/history`,
  `/api/history`. Nothing there mutates and nothing there is a secret, because
  nothing on this wire is: what the page shows is what the relay already
  broadcasts to every peer, and `join.md` states that in full before anybody
  joins. Not public: the archive's own listener, the relay's admin path, the data
  directories, the verifier store, SSH.
- **The cost, named.** The status page is the largest single egress term in the
  service — larger than the game traffic — at ~32 GB/month per continuously open
  browser tab uncompressed, ~4 GB gzipped. It is inside the bundle's included
  transfer, and it is the first time this page has ever been reachable by anybody
  but the owner.

**B28's admin path stays off.** `MULTIVERSE_RELAY_ADMIN_LISTEN` is deliberately
absent from the generated environment file; the listener is compiled in and bound
to nothing until an owner-level act turns it on, and off loopback it also demands
TLS. `provision.sh --only verify` checks that it is still unset.

## 6. The owner's remaining manual steps

Everything in this list is a console act, a purchase or a decision. **No script
here can do any of it**, and none of it has been done.

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
6. **Register the domain and point it.** Any registrar. Create an **A record**
   for the name at the static IP, TTL 300 while setting up. If an **AAAA** record
   exists it must point at the same instance, or the ACME validator will reach
   the wrong host over IPv6. **The name is permanent for the run.**
7. **Open four ports in the Lightsail console's own firewall** — Networking →
   IPv4 Firewall: **22** (SSH), **80** (ACME), **443** (the map), **8443** (the
   status page). ufw is the second of two firewalls and the console's is the one
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

- **External reachability cannot be tested from the instance.** `/etc/hosts` pins
  the domain to loopback so the archive can dial the relay by name over a
  certificate that names only the domain — on Lightsail the public address is
  NAT'd and is not reachable from the instance itself. Every check therefore
  takes the inside path. **The first proof that a stranger can reach the map is a
  stranger reaching the map**, which is WP8's.
- **The forward receipt is not here.** B26 ships from the parallel arc.
  `RESTART-POLICY.md` §1.2 describes the world *without* it and says so; the
  paragraph is marked and should be revised when B26 deploys.
- **The measured per-migration cost of the receipt at rate** is WP3's done-when
  and belongs to that arc plus WP8, not to this one.
- **Nothing here has been executed.** `bash -n` clean and `systemd-analyze
  verify` clean (§8), and that is the whole of the verification available without
  an instance. Every literal command is written to be read before it is run, and
  `provision.sh --dry-run` exists so that the first run is a rehearsal.
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
| `bash -n` on all six scripts | clean |
| `systemd-analyze verify` on all six units | clean on systemd 255 — the only notices are "command not found" for paths that exist on the instance and not here |
| `nginx -t` | **not run** — no nginx on the development machine. It runs inside `provision.sh` before every reload, so a bad render never reaches a live listener |
| `shellcheck` | **not run** — not installed here. The scripts are written to its rules: `set -uo pipefail` (`-e` only where a partial run would be worse than a stop), every expansion quoted, `read -r`, `$(...)`, no unquoted globs, arrays for argument lists, and one annotated `disable=SC2086` where the operator's value carries its own arguments |

Behavioural checks, against the real binaries built by `ship.sh` from this
commit, on scratch ports and a scratch data directory. **The living deployment
was not touched** and every build and process ran under `nice -n 19`, per the
rig's own constraint:

| What was proven | How |
|---|---|
| **The cross-compile is real.** `linux/amd64` and `linux/arm64`, both **statically linked** | `ship.sh --build-only`, then `file` |
| **The generated environment files drive both binaries with no arguments at all** | Both started from `relay.env` / `archive.env` alone; `ExecStart` carries nothing |
| **The relay serves TLS on the env-file port with the env-file pair**, and answers `/healthz` | `curl --cacert` |
| **Plaintext against the TLS listener is refused** | `Client sent an HTTP request to an HTTPS server` |
| **The bootstrap mint is what lets the relay start at all.** An empty credential store makes it refuse to serve and exit 1 — which is why `bootstrap` precedes `systemd` | Observed, then re-run in the correct order |
| **The archive subscribes with the bootstrap-minted `subscribe` credential** read from `MULTIVERSE_CREDENTIAL_FILE` | `relayConnected: true`, `haveStatus: true` |
| **A certificate rotation is survived with no restart and no signal** — the premise the whole TLS design rests on | Replaced both PEMs under the running relay; the served fingerprint changed on the next handshake and `/healthz` never faltered |
| **`monitor.sh`'s thirteen checks run, escalate and recover** — including the lane-bypass persistence counter firing on exactly the third consecutive pass, and the replay projection reproducing the options document's model (7.43 M records → ~9.4 GB peak) | A synthetic `/api/status` server and a synthetic state directory |
| ↳ **The replay check's constants changed on 2026-08-12 and have NOT been re-rehearsed.** It now projects both terms and alerts on the larger, so the same 7.43 M records project ~2.1 GB (resident) rather than ~9.4 GB. The escalate-and-recover behaviour around it is untouched | — |
| **`monitor.sh`'s error counter survives the numbered log rotation** | Appended errors, rotated `relay.log` → `relay.log.1`, confirmed no double count and no missed lines |
| **`backup.sh` produces, prunes and round-trips** — identity snapshots with checksums, `MV_BACKUP_KEEP` pruning, gzip of the ledger that `gunzip | diff` matches, the hardlink genome snapshot, and the `auto` guard correctly refusing to copy a ledger onto a filesystem below its free-space floor | Synthetic data directory |

One observation worth recording rather than filing as a defect: on a six-slot
fixture the status frame compresses ~10×, but on the empty test map the frame was
573 bytes and came back uncompressed — correctly, because the archive leaves
anything under `gzipMinBytes = 1400` alone. Compression is a real term at real
map sizes and a no-op at trivial ones.

**Nothing was run against a cloud account. No account exists.**
