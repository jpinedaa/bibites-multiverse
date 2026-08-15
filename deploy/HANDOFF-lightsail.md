# Handoff — standing up the hosted relay and archive on AWS Lightsail

**Who this is for.** A fresh session with no memory of the conversation that produced the hosting
kit, working alongside the project's owner to turn `deploy/` from a reviewed set of scripts into a
running public service. It assumes you are competent and that you know nothing about this project.

**What this document is, and what it is not.** It is the *sequence*: the order the phases happen
in, the gate each one has to pass before the next begins, what "it worked" looks like in observable
terms, and what to do when it does not. It is **not** a second copy of the commands.
[`deploy/README.md`](README.md) is the kit's own index and it holds every literal command, the
fifteen provisioning phases, the parameter surface and the owner's twelve console steps. Read it
first and keep it open; where this document and the README disagree, **the README is the artifact
that ships and it wins.**

**Read, in this order, before you touch anything:**

1. [`deploy/README.md`](README.md) — the index, §3's execution order, §6's owner steps.
2. [`deploy/deploy.env.example`](deploy.env.example) — every variable with its reasoning beside it.
3. [`deploy/SIZING.md`](SIZING.md) §4 and §5 — the memory arithmetic, which is the only open
   decision left.
4. [`deploy/RESTART-POLICY.md`](RESTART-POLICY.md) §1 — what each kind of restart costs.
5. [`deploy/WIND-DOWN.md`](WIND-DOWN.md) and [`deploy/ANNOUNCEMENT.md`](ANNOUNCEMENT.md) — the
   ending, and the text that has to exist before anybody joins.
6. `m5_tracking.md`, the WP3 row and *Orchestration protocol* — what is done, and the rules the
   doing follows.

---

## 1. What this is, where the project stands, and what "done" looks like

### The thing being built

The Bibites Multiverse is a map: a set of independently-run Bibites worlds connected by a relay, so
that organisms which walk into a portal in one person's world arrive in somebody else's. Today it
runs on the owner's own machines — five local worlds plus one "far end" second computer, on a LAN
relay with a private certificate authority and a hand-carried bundle. That fleet is called *the
living deployment* and it keeps running throughout; nothing in this handoff touches it.

What this handoff stands up is the **public** version of the same thing: one AWS Lightsail instance
in US East running two processes co-hosted —

- **nginx**, which terminates TLS on port 443 and routes the website and relay;
- **the relay** (`multiverse-relay`), which speaks Contract B at
  `wss://bibitesmultiverse.com/contract-b/v4`; and
- **the archive** (`multiverse-archive`), which subscribes to the relay, writes the permanent
  record of every crossing, and draws the website at `https://bibitesmultiverse.com/`.

Strangers join by being handed a **join string** out of band. There is no account system, no email
and no password reset: the relay keeps a verifier, the secret is printed once, and whoever holds the
line *is* that world on the map.

### Where the project stands

The service went live on AWS Lightsail on 2026-08-14. The instance uses static address
`3.229.27.163` in `us-east-1a`. Cloudflare serves both A records as DNS-only records.
The certificate, alarms, snapshots, reboot test, alert test, and identity restore test passed.

The public website update from PR #1 went live on 2026-08-15. The archive runs clean revision
`87dc4a2`. The root path now serves the landing page, and `/live` serves the complete console.
The archive-only restart began at 02:54:22 UTC and reconnected on the third one-second probe.
All 14 service checks passed. Mobile Lighthouse scored 100 in all four audited categories.
The map returned with five live slots, no dark slots, and one hole. The relay did not restart.
Its on-disk binary is from the same revision. The running relay keeps its previous executable until
the next planned relay restart.

### What "done" looks like

Done is a public relay and archive that a stranger can join, with all six of Design Question 2's
operational obligations satisfied — DQ2's argument is that *the hosted relay is a service and not a
process*, and each obligation has a file in this kit:

| DQ2's obligation | Satisfied by | What proves it, live |
|---|---|---|
| A supervisor — restart on exit, start on boot | `systemd/multiverse-{relay,archive}.service` | `systemctl is-enabled` says `enabled` for both, and both survive a deliberate reboot |
| Monitoring that speaks when nobody is looking | `monitor.sh` + `multiverse-monitor.timer` | `monitor.sh --test` delivers an alert to the owner's phone, and the daily heartbeat arrives |
| Backup of the irreplaceable files | `backup.sh` + `multiverse-backup.timer`, plus Lightsail automatic snapshots | `backup.sh --list` shows an identity snapshot under an hour old, and a restore has been rehearsed |
| A written restart policy | `RESTART-POLICY.md`, installed on the box at `/opt/multiverse/deploy/` | It is readable at 03:00 on the machine that is misbehaving, which is the whole point of putting it there |
| A name, a certificate and its renewal | `provision.sh --only tls`, `tls-deploy-hook.sh`, `nginx/` | A browser reaches `https://bibitesmultiverse.com/` with no warning, and `certbot.timer` is active |
| D24's announced period and its ending | `WIND-DOWN.md`, `ANNOUNCEMENT.md` | Real dates in `MV_PERIOD_START`/`MV_PERIOD_END`, and the period stated on the release page **before** the first stranger's join string is issued |

Plus the two things WP3 owes beyond DQ2: Decision 3's retention rule with the arithmetic a hoster
sizes a volume from (`SIZING.md`), and the deliberate publication of the status page
(`nginx/multiverse-20-status.conf`).

**External reachability must be proven outside the box.**
`provision.sh` writes a `/etc/hosts` entry pinning `bibitesmultiverse.com` to `127.0.0.1`, because
the archive dials the relay *by name* over a public certificate and Lightsail's public address is
NAT'd and unreachable from the instance itself. Every check the box runs against its own name
therefore takes the inside path. The website and relay path passed off-box checks after deployment.

---

## 2. The decided state you inherit

Every row below is a decision already taken. Do not re-open one without the owner; do verify each
against the file named, because this table is a summary and the files are the record.

| What | The decision | Decided | Where it lives |
|---|---|---|---|
| **Provider** | **AWS Lightsail, US East**, one instance running relay and archive together | owner, 2026-08-12 | `wp3_hosting_options.md` "The single recommendation"; `m5_tracking.md` WP3 |
| **Name** | `bibitesmultiverse.com`, with `status.bibitesmultiverse.com` on the same certificate | owner, 2026-08-14 (commit 5d5c19c) | `MV_DOMAIN`, `MV_CERT_EXTRA_NAMES` |
| **Registrar** | Cloudflare, owner-registered | 2026-08-14 | [`deploy/HANDOFF-domain.md`](HANDOFF-domain.md) |
| **Public HTTPS** | **443** for the website and `/contract-b/` relay path | owner, 2026-08-14 | `MV_RELAY_PORT`; README §4 |
| **Relay backend** | `127.0.0.1:8795`, reached only through nginx | owner, 2026-08-14 | `MV_RELAY_BACKEND`; README §4 |
| **ACME** | certbot, **HTTP-01, webroot**, through nginx on port 80 | 2026-08-12 | `MV_ACME_MODE=webroot`; README §4 |
| **Retention (Decision 3)** | **The ledger forever; genome blobs pruned to a horizon.** `MV_RETENTION=prune-genomes`, `MV_ARCHIVE_GENOME_HORIZON=720h` (30 days) | owner, 2026-08-12 | contract §23 B33–B34; `WIND-DOWN.md` §4 arm B |
| **Archive memory** | `MV_ARCHIVE_GOMEMLIMIT=5GiB` as a ceiling against regression, `MV_ARCHIVE_MEMORY_HIGH` empty, `MV_SWAP_GB=0` | measured, 2026-08-12 | `SIZING.md` §4; the matrix is in `wp3_hosting_options.md` |
| **Announced period (D24)** | **2026-08-14 through 2026-11-14**, extendable by announcement, never silently shortened | owner, 2026-08-14 | `WIND-DOWN.md`; live `deploy.env` |
| **Alert channel** | **ntfy.sh**, tested without recording its capability URL | verified, 2026-08-15 | live `deploy.env` |
| **Admin path** | **Off.** `MULTIVERSE_RELAY_ADMIN_LISTEN` is deliberately absent; `provision.sh --only verify` checks it stays unset | 2026-08-11 | README §5; `relay.env` |

The example file keeps two owner-specific values open for a fresh deployment.
The live parameter file contains both real values. Do not copy its secrets into this repository.

- `MV_ACME_EMAIL=CHOOSE@example.org` — **`provision.sh` refuses to run until this is real.** It is
  Let's Encrypt's expiry warning address, the last line of defence behind `monitor.sh`'s
  certificate check.
- `MV_ALERT_URL=` — empty. `monitor.sh` will log "alert dropped" and reach nobody.

### The live size and the arithmetic behind it

The live instance uses the $12 `small_3_0` bundle. Resize when the monitor's RAM tripwire fires.
`SIZING.md` §5 endorses exactly this plan —
"a third plan exists and is cheaper than either row" — because Lightsail resizes through a snapshot,
which converts a sizing error from an outage into a bill plus a few minutes of announced downtime.

| Bundle | Spec | Three months | On the trial list? |
|---|---|---|---|
| **$12** | 2 vCPU, **2 GB**, 60 GB SSD, 3 TB transfer | $36, or **$0** under the 90-day trial | Yes |
| $24 | 2 vCPU, 4 GB, 80 GB SSD, 4 TB | $72 | No |
| $44 | 2 vCPU, 8 GB, 160 GB SSD, 5 TB | $132 | No |

**The 90-day free trial is worth exploiting and worth not depending on.** New accounts get 90 days
free on the $5/$7/$12 Linux plans, **one bundle per account**, for accounts that started using
Lightsail on or after 2021-07-08. D24's announced period is three months and the $12 bundle is on
that list, so the announced run can cost **$0 in compute**. Those terms were fetched 2026-08-11 and
are quoted in `wp3_hosting_options.md`; **re-check them on the pricing page at the moment of
purchase** and verify eligibility *before* announcing anything.

**Disk is comfortable on the $12 bundle, and memory is the term that binds.** `SIZING.md` §2's rule
is *durable bytes per day = 56 MB × S + 1.6 MB × slots*, where **S is the sum of achieved time
scales across the map** and not the peer count — every durable term is produced by *simulated* time,
so one participant at ×20 costs as much as twenty at ×1. At the exit-test bar (five peers at ×1,
S = 5) that is 0.29 GB/day, or 26 GB over ninety days if nothing is ever pruned.

The 720-hour horizon changes the shape of that: it turns the genome store's growth from a total into
a rate, so its steady state is thirty days' worth rather than ninety. Working §2's terms through with
the horizon applied — this arithmetic is derived here, not quoted from the kit — the day-90 footprint
is roughly **19 GB at five peers** (7.2 GB of ledger, 6 GB of genome store, 0.9 GB of metrics, 1.2 GB
of rotated logs, ~4 GB of OS image) and roughly **40 GB at twelve peers at ×1**. Both fit inside
60 GB. What does not fit is the speed slider: `SIZING.md` §3's twelve-peers-at-×5 row is 303 GB in
ninety days, and the ledger term alone would pass 60 GB before day 65. **D23 deferred the speed
control surface to M6**, so the operator can watch a participant run their world at seven times
everyone else's rate and can do nothing about it — Risk 9 recorded that as a measurement-quality
cost, and `wp3_hosting_options.md` is the first document to say it is also a billing cost. Lightsail
grows a volume online, so this is a bill rather than an outage, and `monitor.sh`'s disk check reports
days-to-full from the box's own last 24 hours long before it matters.

**Memory is where the honest caveat lives, and it is stated wrongly in one place in the repository
already — correct it wherever you find it.** Pruning genome *blobs* does not bound the archive's
memory, because the resident set grows with ledger *records* and the ledger is kept forever. The
kit sizes conservatively from the living deployment's measured **0.30 KB resident per ledger
record** (the streamed build's own retained set measured 0.16 KB/record immediately after replay,
but no streamed archive has yet served live traffic for a day, and `SIZING.md` §4 is explicit that
the deployment's figure is the one that has). At the exit-test bar's 242,000 records/day:

| Milestone on a 2 GB box | Records | Day of the run | Source |
|---|---|---|---|
| `monitor.sh` **replay WARN** — decide or size up | ~4.3 M | **~day 18** | `monitor.sh`, ratio ≥ 0.75 × 0.85 of MemTotal+swap |
| `monitor.sh` **replay CRIT** — do not restart the archive | ~5.8 M | **~day 24** | `monitor.sh`, ratio ≥ `MV_REPLAY_HEADROOM` = 0.85 |
| Resident crosses 2 GB — the box stops holding the archive | 6.7 M | **day 28** | `SIZING.md` §4 |
| Streamed *replay peak* crosses 2 GB | ~11.4 M | day 45 | `wp3_hosting_options.md`, the streaming row |

**"Roughly day 45" is the replay term, and since 2026-08-12 the replay is no longer the binding
one.** The resident term is larger, `monitor.sh` alerts on the larger of the two, and the box's
`MemTotal` on a "2 GB" instance is around 1.9 GiB rather than a round 2,048 MB — which is why the
tripwire fires in week three or four, not week seven. If the streamed build's 0.16 KB/record does
survive a day of live service, every date above moves out by about 80% (`SIZING.md` §4 says so
explicitly, and calls the gap "worth a day of somebody's attention"), which puts the hold wall near
day 50 — but the kit deliberately sizes the conservative way, because a tripwire that fires early
costs a look and one that fires late is the outage it existed to prevent.

**`m5_tracking.md`'s WP3 row currently says "a 2 GB box holds the exit bar to ~day 45", which
contradicts `SIZING.md` §4.** That is a real contradiction inside the repository, not a rounding
difference, and it should be corrected to the conservative figure when you next touch that row (§7).

**What this means operationally:** the $12 bundle carries the run comfortably for the first two to
three weeks, the monitor tells you in advance, and the resize is a planned act. The resize procedure
is in `backup.sh --restore-help` under *RESTORING THE WHOLE INSTANCE*: snapshot, create a larger
instance from the snapshot, **attach the same static IP** (the A record must not have to change —
the name is in every join string), then re-run `provision.sh`, which is idempotent and will find its
own files. Two costs to state when you plan it: the $24 and $44 bundles are **not** on the free-trial
list, so a resize starts the bill; and it costs an announced archive restart, which is
`RESTART-POLICY.md` §1 case 4.

---

## 3. The division of labour

### What you (the receiving session) can do

- Edit files in this repository, including `deploy.env` templates, the tracking row and the
  announcement text.
- Run `deploy/ship.sh` on the development machine — it cross-compiles and `scp`s, and it touches no
  cloud API. **Run it under the rig's constraint:** `ship.sh` already wraps `go build` in
  `nice -n 19`, because this host also runs the living deployment and unniced load on it has twice
  reproduced a sidecar session storm.
- Run every script in the kit over SSH **once the owner has given you access to the instance**:
  `provision.sh` (including `--dry-run`), `monitor.sh`, `backup.sh`, `issue-join.sh`.
- Verify anything, read logs, read `/api/status`, and record what you found.

### What only the owner can do

Everything here is a console act, a purchase or a decision. **No script in this kit can do any of
it**, and README §6 is the authoritative list of twelve.

1. Create or sign in to the **AWS account**, and hold the payment method.
2. **Verify free-trial eligibility** — it depends on account history you cannot see.
3. **Decide the bundle** (`MV_BUNDLE`), with §2's arithmetic in front of him.
4. **Create the instance** — Linux/Unix, OS Only, **Ubuntu 24.04 LTS**, region **US East**.
5. **Create and attach the static IP.**
6. **Register the domain and create the DNS records** (the sibling handoff owns this).
7. **Open the ports in the Lightsail console firewall.**
8. **Create the ntfy topic** and subscribe to it — it is a capability URL and belongs in a 0640 file,
   never in a document, never in a commit, and never in your transcript.
9. **Create the Lightsail metric alarm** (instance status check failed → email).
10. **Enable automatic snapshots** — the offsite half of `backup.sh`, and the only thing that
    genuinely protects the record.
11. **Hand over SSH access**, or run README §3's steps himself.
12. **Announce the period** and publish the release.

### The rule that governs the boundary

**You must not create billable resources.** Do not create an AWS account, do not create an instance,
do not attach an IP, do not enable snapshots, do not register a domain, and do not run anything that
provisions cloud capacity — even if you are handed credentials that would let you. Your job is to
prepare, sequence, verify and record; the owner's job is to spend. If a phase is blocked on a
console act, say so plainly and stop rather than routing around it.

The same boundary applies inside the repository: **agents do not commit.** Report the paths you
changed and a proposed commit message, and let the orchestrator stage and commit — this checkout is
shared with parallel sessions and concurrent staging is how one session's index picks up another's
half-finished work (`m5_tracking.md`, *Orchestration protocol*).

---

## 4. The eleven ways this goes wrong

Read these before the runbook. Each one is cheap to avoid and expensive to hit, and each is
referenced by number at the phase where it bites.

**T1 — Attach the static IP BEFORE creating the A record.** A Lightsail instance's *default* public
address changes when it is stopped and started. If the A record is created against that address, the
map moves out from under its own name at the first stop, and the name is in every join string ever
minted. Attach the static IP first, then point DNS at the static IP. (README §6 step 5.)

**T2 — Both A records must be "DNS only" (grey cloud) at Cloudflare.** Cloudflare defaults new
proxied records to the orange cloud. A proxied record terminates TLS at Cloudflare, which breaks the
`wss://` path in every join string — nginx's origin certificate is the identity a sidecar expects —
and hides the origin from the ACME validator. Both `bibitesmultiverse.com` and
`status.bibitesmultiverse.com` get their own A record at the same static IP, and both must be grey.

**T3 — `status.bibitesmultiverse.com` must be on the certificate at issuance.** It already is, via
`MV_CERT_EXTRA_NAMES`. A name added later needs a re-issue, and a re-issue spends an ACME rate-limit
slot. Keep it because it costs nothing at issuance and cannot be added for free afterwards.

**T4 — Open the ports in the Lightsail console's own firewall, not only in `ufw`.** `provision.sh`
configures `ufw`, which is the *second* of two firewalls; the console's is the one the internet meets
first, and a new instance opens only 22 and 80 by default. Three rules: **22** (SSH), **80** (ACME
HTTP-01), and **443** (HTTPS). If the instance is dual-stack, repeat every
rule under the IPv6 firewall. And if an **AAAA** record exists anywhere it must point at this same
host — the ACME validator prefers IPv6, so a stale AAAA fails issuance while every IPv4 test on the
box passes. The simplest safe answer is to create no AAAA record at all.

**T5 — Run `provision.sh --dry-run` first.** It changes nothing — every mutation in the script goes
through one `run` helper that prints under `--dry-run` — and it checks the A record against the
instance's own metadata address, which is what catches a proxied or mis-pointed record *before* ACME
burns a rate-limit attempt. It also checks the staged binaries and every value in `deploy.env`.

**T6 — The relay refuses to serve with an empty credential store.** It exits 1 with "the credential
store holds no credentials, so this relay can admit nobody". This is why `provision.sh`'s
`bootstrap` phase — which mints the archive's `subscribe` credential — runs *before* the `systemd`
phase. The ordering is load-bearing, not cosmetic: a run that started the units first would leave a
unit in `failed` and an operator reading systemd instead of the relay's own log. Do not reorder the
phases, and do not run `--only systemd` on a fresh box before `--only bootstrap`.

**T7 — Minting a join string requires a relay restart.** `--mint-credential` is a *startup* command:
it opens the data directory, acts and exits. A serving relay has already read `peers.json` into
memory and writes it back whole on its own next mint, so minting against a live relay makes a second
writer — the new peer gets HTTP 401 until a restart, and a later handover silently erases the
credential. `issue-join.sh` takes the restart deliberately and **batches**: one restart for five
participants, not five restarts. B28's admin path has release, handover and evict but **no mint
act**, so there is no online alternative; that is the contract amendment to write if onboarding at
rate ever hurts.

**T8 — `MV_ACME_MODE` stays `webroot`.** In `dns` mode `provision.sh` refuses rather than guessing a
registrar plugin. Note the nuance the kit predates: the registrar *is* now known (Cloudflare), and
`certbot-dns-cloudflare` exists, so DNS-01 is possible in principle and would let port 80 close.
It is still not supported by the kit as written — the `tls` phase would need adapting and an API
credential would need storing at `/etc/multiverse/dns-credentials.ini` — so **this run uses webroot**
and port 80 stays open. Treat the switch as a future improvement with its own arc, not a decision to
take under time pressure on provisioning day.

**Three more, found in the kit's own behaviour rather than in its documents. All three have since
been fixed in the kit — each entry below says what the kit now does, because the trap is still worth
recognising when you meet its output.**

**T9 — `provision.sh`'s A-record check is only trustworthy on the first run. FIXED: the check now
says so itself.** The `envfiles` phase appends `127.0.0.1 bibitesmultiverse.com` to `/etc/hosts`
(deliberately — it is what keeps the archive's dial to the relay on loopback), and on any subsequent
run `preflight`'s `getent ahostsv4` reads that entry before it reads DNS. It used to report that as
a mismatch against the instance's public IPv4, which reads like a failure and is not one.
**`preflight` now detects the `127.` answer and prints it as the pin it is** — *"THIS INSTANCE'S OWN
/etc/hosts PIN, NOT DNS"* — with the off-box command to use instead, and it names the public IPv4
that command must return. A *different* wrong address is still a warning, unchanged. Either way the
rule stands: verify the real record from *off* the box — `dig +short bibitesmultiverse.com @1.1.1.1`
from the development machine — and never conclude anything about DNS from inside the instance.

**T10 — `MV_EXPECTED_PEERS=1` would page you before anybody had joined. FIXED: it ships at 0, and
the check says what 0 means.** `monitor.sh` reports `peers CRIT` when live slots fall below the
floor, and a freshly provisioned map has zero — so the first tick after provisioning was a CRIT, and
an operator whose first-ever alert is a false one mutes the channel while he is still deciding
whether to trust it. **The floor is now 0 by default in both `deploy.env.example` and `monitor.sh`.
0 means NO FLOOR: the live count cannot be too low and the peers check watches dark slots only.**
That would be the wrong value forever, since "peers lost" is one of WP3's own done-when clauses, so
the `peers OK` line carries `NO FLOOR SET (MV_EXPECTED_PEERS=0)` and the instruction to raise it on
every run until somebody does. **Raise it at Phase 15**, once the first peers hold slots.

**T11 — `/etc/multiverse/deploy.env` must be group `multiverse`, and README §3's literal command did
not make it so. FIXED in four places, so it cannot regress from any one of them.** This was a defect
in the kit, verified rather than suspected, and it silently disabled two of DQ2's six obligations.
`deploy.env.example`'s header states the intended mode as *"0640, root:multiverse"*, but README §3
step 3 was `sudo install -m 0640 … /etc/multiverse/deploy.env` with no `-o`/`-g`, and `install` run
under `sudo` gives the file **root:root**. Nothing chowned it afterwards — the `directories` phase
sets the group on the *directory* only, and `relay.env` and `archive.env` are written explicitly as
`root:$MV_GROUP` so they were never affected. But `multiverse-monitor.service` and
`multiverse-backup.service` both run `User=multiverse`, and both scripts begin with a hard
`[ -r "$ENV_FILE" ] || exit 2`. Left as the README wrote it, **the monitor and the backup timer
exited 2 on every tick**: no alerts, no heartbeat, no identity snapshots, and nothing announcing it.
What the kit does now:

- **README §3 step 3** creates the group if it does not exist and installs the file as
  `install -o root -g multiverse -m 0640 …`, then prints `ls -l` so the ownership is read rather
  than assumed.
- **`provision.sh`'s `envfiles` phase** chowns and chmods the file to `root:$MV_GROUP 0640` itself,
  so a hand copy is repaired by the ordinary run.
- **`provision.sh`'s `verify` phase** proves it by *reading the file as the `multiverse` user*
  (`runuser -u multiverse -- test -r …`), not by inspecting the mode — the mode can be right while
  the group is wrong — and prints the one-line remedy when it fails.
- **`monitor.sh` and `backup.sh`** now distinguish *missing* from *unreadable* in their exit-2
  message, so the next person meets the cause instead of the symptom.

The one-line remedy, if you ever meet it on an older box:

```sh
sudo chown root:multiverse /etc/multiverse/deploy.env && ls -l /etc/multiverse/deploy.env
```

---

## 5. The runbook

Each phase names **who acts**, its **gate** (what must be true before it starts), the **act**, the
**verification** that it worked in observable terms, and **what to do when it fails**. The literal
commands live in README §3 and §6; this sequences them.

### Phase 0 — The domain exists

**Who:** the owner, with the sibling session. **Gate:** none; this can run in parallel with
Phases 1–4, and it must complete before Phase 5.

`bibitesmultiverse.com` is registered at Cloudflare and its zone is under the owner's control. The
detail belongs to [`deploy/HANDOFF-domain.md`](HANDOFF-domain.md).

**Verify:** `dig +short NS bibitesmultiverse.com` returns Cloudflare nameservers, and the zone is
visible in the Cloudflare dashboard.

**When it fails:** a brand-new `.com` can take minutes to an hour to be resolvable everywhere. This
is the one phase where waiting is the correct action. **Do not start Phase 10 while DNS is still
settling** — an ACME failure here is expensive (§6) and a delay is free.

### Phase 1 — AWS account and free-trial eligibility

**Who:** the owner only. **Gate:** none.

Create or sign in to the AWS account (`console.aws.amazon.com`), and check Lightsail free-trial
eligibility: 90 days free on the $5/$7/$12 Linux plans, one bundle per account, for accounts that
started using Lightsail on or after 2021-07-08.

**Verify:** the Lightsail console loads, the region selector offers US East, and the trial's terms
are read off the current pricing page rather than off this document.

**When it fails:** if the account is not eligible, the $12 bundle costs $36 over the announced
period. That changes the budget, not the plan. **Do not announce a period on the assumption of a
$0 bill** — verify first, then announce.

### Phase 2 — The bundle decision

**Who:** the owner, informed by §2 above and `SIZING.md` §4–§5. **Gate:** Phase 1.

**Plan of record: the $12 bundle** (2 GB / 60 GB / 3 TB), resized when `monitor.sh`'s replay check
goes WARN.

**Verify:** the answer is written into `MV_BUNDLE` in Phase 8, so that the box says what it was
sized for.

### Phase 3 — The instance

**Who:** the owner only. **Gate:** Phase 2.

Lightsail → Create instance → **Linux/Unix** → **OS Only → Ubuntu 24.04 LTS** → region **US East** →
the chosen bundle. Note the SSH key: the scripts assume the Lightsail default user `ubuntu` and a
key the owner holds.

**Verify:** `ssh ubuntu@<address>` succeeds and `lsb_release -a` reports Ubuntu 24.04.

**When it fails:** almost nothing here fails. If the wrong OS was chosen, delete and recreate —
`provision.sh` refuses on anything without `apt-get` and systemd, and it is cheaper to recreate than
to argue with it.

### Phase 4 — The static IP — **before DNS** (T1)

**Who:** the owner only. **Gate:** Phase 3.

Lightsail → Networking → Create static IP → attach it to the instance.

**Verify:** the instance's public address in the console equals the static IP, and `ssh` to that
address still works.

**When it fails:** if an A record was already created against the ephemeral address, fix the record
now rather than later; it is cheap before ACME has issued and it is a re-issue afterwards. Also note
for later: **do not release the static IP during a resize** — Lightsail bills a detached static IP,
and more importantly the A record depends on it.

### Phase 5 — DNS records, grey cloud (T2, T3, T4)

**Who:** the owner, with you verifying. **Gate:** Phases 0 and 4.

Two A records at Cloudflare, both **DNS only / grey cloud**, both at the static IP, TTL 300 while
setting up:

- `bibitesmultiverse.com` → the static IP
- `status.bibitesmultiverse.com` → the same static IP

Create no AAAA record.

**Verify, from the development machine and not from the instance:**

```sh
dig +short bibitesmultiverse.com @1.1.1.1
dig +short status.bibitesmultiverse.com @1.1.1.1
dig +short AAAA bibitesmultiverse.com @1.1.1.1     # expect nothing
```

Both must return the static IP and nothing else. **If they return Cloudflare addresses (typically
`104.x` or `172.67.x`), the record is proxied** — that is T2, and it must be fixed before Phase 10.

**When it fails:** a wrong or proxied record found here costs a TTL. The same record found in
Phase 10 costs an ACME attempt. Spend the time here.

### Phase 6 — The Lightsail console firewall (T4)

**Who:** the owner only. **Gate:** Phase 3.

Networking → IPv4 Firewall: **22**, **80**, and **443**. Repeat under IPv6 Firewall if the
instance is dual-stack.

**Verify, from off the box** — port 80 will refuse until nginx is up in Phase 10, so at this stage
the observable is the console's own rule list. After Phase 10, test ports 80 and 443 from the
development machine.

**When it fails:** a closed 80 is the classic cause of an ACME failure that looks like a DNS problem.
`ufw` being open proves nothing about the console.

### Phase 7 — Ship the binaries and the kit

**Who:** you, from the development machine. **Gate:** SSH access.

README §3 steps 1–2. `deploy/ship.sh --host <ip>` cross-compiles `relay`, `archive` and `ringstat`
for both `amd64` and `arm64`, verifies the relay is **statically linked** (a cgo build would fail on
a different libc), `scp`s them to `/home/ubuntu/multiverse-stage/` and re-verifies the checksums on
the far side. Then `scp -r deploy ubuntu@<ip>:/home/ubuntu/multiverse-kit`.

**Verify:** `ship.sh` prints matching `sha256sum -c` output from the instance, and
`ls /home/ubuntu/multiverse-stage` on the box shows six binaries plus `SHA256SUMS`.

**Worth knowing:** because these binaries are built from current `HEAD`, the fresh instance starts
with every capability the living deployment had to bank against an archive restart — the streamed
replay, the genome horizon, the status page's gzip, B30's deny list, and the `limits` /
`minContractVersion` keys on `/api/status`. Nothing is owed against a restart on this box.

**When it fails:** "no go on PATH" means you are on the wrong machine. A non-static relay means
`CGO_ENABLED=0` was overridden. A checksum mismatch on the far side means the transfer, not the
build — re-run it.

### Phase 8 — Fill `/etc/multiverse/deploy.env`

**Who:** you, on the instance. **Gate:** Phase 7.

README §3 step 3 copies `deploy.env.example` to `/etc/multiverse/deploy.env` at mode 0640,
`root:multiverse`. Fill in:

| Variable | Value | Why it matters here |
|---|---|---|
| `MV_ACME_EMAIL` | the owner's address | `provision.sh` refuses on the placeholder |
| `MV_ALERT_URL` | the ntfy topic from README §6 step 8 | a capability URL — 0640, never in a document or a commit |
| `MV_BUNDLE` | `2gb` (per Phase 2) | informational; the box's record of what it was sized for |
| `MV_PERIOD_START` / `MV_PERIOD_END` | real dates, three months apart | `preflight` now warns on the placeholder, on a date that is not one, and on an end before its start — and prints the period's length in days. It is still a warning, not a stop: no command here reads them, and Phase 14 is the gate |
| `MV_EXPECTED_PEERS` | leave it at `0` (T10) | 0 means no floor: dark slots only. Raise it at Phase 15 |

`MV_DOMAIN`, `MV_CERT_EXTRA_NAMES`, the ports, the retention rule and the memory verdict already
carry their answers.

**Verify:**

```sh
grep -c CHOOSE /etc/multiverse/deploy.env      # 0
grep -n 'YYYY-MM-DD' /etc/multiverse/deploy.env # nothing
ls -l /etc/multiverse/deploy.env                # -rw-r----- root multiverse   <- see T11
```

The ownership line is the one people skip, and it is the one that used to be wrong (T11). README §3
step 3 now installs the file as `root:multiverse` and creates the group first if it has to, so this
should read correctly the moment the file exists; `provision.sh` re-asserts it in `envfiles` and
proves it in `verify`. If this `ls` says `root root`, fix it before Phase 12 — that is exactly the
state in which the monitor and backup timers fail silently.

### Phase 9 — `provision.sh --dry-run` (T5, T9)

**Who:** you. **Gate:** Phases 5, 6, 7, 8.

```sh
sudo /home/ubuntu/multiverse-kit/provision.sh --dry-run
```

**Verify — read the preflight output line by line, not the exit code:**

- `bibitesmultiverse.com -> <static IP> (this instance)` on the **first** run. On any later run the
  same line reads `-> 127.0.0.1 — THIS INSTANCE'S OWN /etc/hosts PIN, NOT DNS`, which is the pin the
  `envfiles` phase added and **is not a fault** (T9). A *different* address is still T2 and stops
  here. Either way, the record itself is proven from off the box with `dig`, never from this line.
- `retention rule: prune-genomes`.
- `announced period: <start> -> <end> (91 days)`. A `!!` line here means the dates are still
  placeholders or run backwards — provisioning continues, but Phase 14 is blocked until they are real.
- `staged: archive-linux-amd64 archive-linux-arm64 relay-linux-...` — the architecture matching
  `uname -m` must be present.
- The five summary lines: domain, website, relay URL, relay backend, and archive.
- Every subsequent phase prints `[dry-run]` before each mutation and creates nothing.

**When it fails:** every `STOP:` line names its own remedy. The three you are most likely to see are
the ACME email placeholder, a missing stage directory (Phase 7 did not land) and the domain not
resolving (Phase 0/5).

### Phase 10 — `provision.sh`, and the certificate

**Who:** you. **Gate:** a clean dry-run, and DNS verified from off the box.

```sh
sudo /home/ubuntu/multiverse-kit/provision.sh
```

Fifteen phases, idempotent, a few minutes. The two that carry risk are `tls` (the ACME issuance) and
`bootstrap` (T6). The run ends in `verify`, which prints fourteen checks.

**Verify — all checks must read PASS.** They cover the units, timers, two loopback upstreams,
website, status API, unauthenticated relay rejection, parameter-file access, and disabled admin path.
Then load `https://bibitesmultiverse.com/` from outside the instance. It must render without a
certificate warning. A request to `/contract-b/v4` without a credential must return HTTP 401.

**Verify the certificate specifically:**

```sh
sudo certbot certificates                      # names: both, and ~89 days left
ls -l /etc/multiverse/tls/                     # certificate 0640, private key 0600 root:root
systemctl is-active certbot.timer              # active
```

**T11 closes itself now — confirm it rather than perform it.** The `envfiles` phase set the
ownership and the `verify` check above proved the read. One command if you want to see it with your
own eyes:

```sh
sudo -u multiverse test -r /etc/multiverse/deploy.env && echo "monitor and backup can read it"
```

**When ACME fails, stop and read §6 before retrying.** A burned attempt is expensive and the
temptation to retry immediately is the trap.

### Phase 11 — Services up, and up again after a reboot

**Who:** you, with the owner's knowledge. **Gate:** Phase 10 verified.

Restart-on-exit and start-on-boot is DQ2's first obligation and the one whose absence would be most
embarrassing. Prove both:

```sh
systemctl is-enabled multiverse-relay multiverse-archive     # enabled, enabled
sudo reboot
# after it comes back:
systemctl is-active multiverse-relay multiverse-archive nginx
curl -s http://127.0.0.1:8796/api/status | jq '{relayConnected, haveStatus, statusAgeMs}'
```

**Verify:** `relayConnected: true` is the reading that matters — an archive that is running and not
subscribed records nothing and complains to nobody. On a fresh box the replay is empty and the
archive is up in seconds; that will not be true in month three, which is what `RESTART-POLICY.md`
§3 is for.

**Do this reboot now, on an empty map.** Once participants have joined, a reboot is a case 4 restart
with an announcement and a ledger gap attached.

### Phase 12 — Monitoring, and the watcher that survives the box being dead

**Who:** you for the first; the owner for the second and third. **Gate:** Phase 11.

```sh
sudo -u multiverse /opt/multiverse/deploy/monitor.sh --test      # one alert, now
sudo -u multiverse /opt/multiverse/deploy/monitor.sh --verbose   # all thirteen checks
```

**Verify:** the owner's phone or browser tab receives the self-test line. `--verbose` prints one line
per check — units, relay-healthz, archive-healthz, subscribed, status-age, peers, lane-bypass, disk,
errors, cert, replay, gaps, backup, reboot. Expect `backup WARN` until Phase 13, and `peers OK` with
`NO FLOOR SET (MV_EXPECTED_PEERS=0)` on the end of the line — that is the shipped default saying it
is watching dark slots only (T10), and it keeps saying it until Phase 15 raises the floor. Everything
else should read OK.

**If either command exits 2, read which of the two things it says** (T11). `monitor: no
/etc/multiverse/deploy.env` is a file that was never installed; `monitor: … exists but is NOT
READABLE by multiverse` is the ownership defect, the monitor timer has been failing silently since
Phase 10, and the message carries its own `chown` remedy. Fix and re-run.

**Everything in `monitor.sh` runs *on* the box, so if the instance is off it says nothing and
silence is indistinguishable from health.** Two answers, and both are the owner's:

- **A Lightsail metric alarm** — the instance's *status check failed* → notify by email. This is the
  only watcher that survives the box being dead.
- **The daily heartbeat** at `MV_HEARTBEAT_HOUR=9` UTC, whose *absence* is the signal. Tell the
  owner what time to expect it, or it is not a signal to anybody.

And one console act that is not a watcher but belongs in the same visit:

- **Automatic snapshots** — Lightsail → the instance → Snapshots → Enable, pick an hour. This is the
  offsite half of `backup.sh` and the only thing that genuinely protects the ledger and the genome
  store, because everything `backup.sh` writes is on the same disk as the thing it protects.
  Roughly $0.05/GB-month, incremental after the first — carried knowledge, check the pricing page.

**When it fails:** `monitor.sh --test` printing "the alert channel is NOT working" means
`MV_ALERT_URL` is empty or the topic is wrong. Fix `deploy.env` and re-run — the monitor reads the
file on every invocation, so no restart is involved.

### Phase 13 — Backups, with a restore rehearsed while nothing is wrong

**Who:** you. **Gate:** Phase 11.

```sh
sudo systemctl start multiverse-backup.service
sudo -u multiverse /opt/multiverse/deploy/backup.sh --list
/opt/multiverse/deploy/backup.sh --restore-help | less
```

**Verify:** `--list` shows at least one identity snapshot containing `ring.json`, `peers.json` and a
`SHA256SUMS`, and `monitor.sh --verbose` now reports `backup OK` with an age in minutes.

**Rehearse the restore that matters, on the empty map, before anybody has an address to lose.** The
identity tier is the one whose loss costs a message to every participant: losing `peers.json` costs
a slot handover *per peer*, and the relay cannot reprint a join string. Take a snapshot, stop the
relay, restore `ring.json` and `peers.json` **from the same snapshot directory**, start the relay,
and confirm the archive re-subscribes. Doing this once now, with nothing at stake, is worth more
than reading the procedure twice.

**When it fails:** a failed hardlink snapshot of the genome store is survivable (`MV_BACKUP_GENOMES=0`
and lean on the Lightsail snapshot). A failed identity snapshot is not — it is the tier with no other
copy, and it is kilobytes, so investigate rather than work around.

### Phase 14 — The announcement, filled in

**Who:** the owner decides the dates; you fill the text. **Gate:** everything above green, and
**before** any join string leaves the box.

`ANNOUNCEMENT.md` holds the text for six documentation slots and edits nothing itself. Fill
`<DOMAIN>`, `<START>`, and `<END>` from `deploy.env`, and write the same dates back
into `MV_PERIOD_START` / `MV_PERIOD_END` so the box and the announcement agree.

The six slots are: `release/RELEASE-PAGE.md`'s `@@OWNER:ANNOUNCED_PERIOD@@` field;
`docs/participant/join.md`'s "The map, its period, and its restarts" (plus three `<relay-host>`
occurrences); `docs/participant/leave.md`'s "When the map itself ends";
`docs/participant/diagnose.md`'s page address; `docs/error-taxonomy.md` §3.2's `B-4003c`; and
`docs/sidecar-diagnose-spec.md`'s `disk-headroom` arithmetic.

**Two rules that come with D24 and must not be paraphrased:** the period is stated **before** anybody
joins, and the bound may be extended by announcement but is **never silently shortened**.

**Verify:** no `<START>`, `<END>`, or `<DOMAIN>` placeholder survives in any edited
file, and no `> **SLOT — WP3` block remains in the three participant documents.

**When it fails:** the honest failure here is announcing a period the owner has not actually
committed to. If the dates are not settled, stop at this phase. Everything below it is gated on it,
including the release.

### Phase 15 — The first join strings (T7)

**Who:** you or the owner. **Gate:** Phase 14 complete, and — for anybody but the owner — the
release published (§8).

```sh
sudo /opt/multiverse/deploy/issue-join.sh peer-<name>
```

The script prints what it is about to do, prints what the restart costs, asks for a typed `yes`,
stops the relay, mints every requested identity in one restart, brings the relay back, and *then*
prints the join strings. It writes each secret to `/var/lib/multiverse/joins/<peerId>.secret` at
mode 0600 as well as printing it.

**Three things to carry:**

- **Batch.** One restart for five participants. Collect the peerIds before you run it.
- **A peerId is permanent.** Slot numbers are never reused, so a peerId that claims a slot owns that
  address forever.
- **Delete each secret file once its owner has applied it** — `sudo shred -u
  /var/lib/multiverse/joins/<peerId>.secret`. The operator's machine has no further use for another
  person's identity.

**Verify:** `issue-join.sh --check` lists the credential and its grant; after the participant
connects, `/api/status` shows their slot live and `monitor.sh --verbose` reports `peers OK`. **Then
raise `MV_EXPECTED_PEERS` to match the map** (T10) — it ships at 0, which means no floor, and until
you raise it the `peers OK` line is telling you so on every run. This is the act that makes "peers
lost" watched.

**When it fails:** "ALREADY HOLDS A CREDENTIAL" means the identity exists and its join string cannot
be reprinted; the only recovery is `--handover-slot <n>=<newPeerId>`, which mints for a *new* peerId
and drops the old, and the participant has to re-apply. That is a message to a person, so avoid
needing it: back up the verifier store (`sudo systemctl start multiverse-backup.service`) after
every issuance.

---

## 6. When something fails

### ACME — the expensive one

**The rate limits are real and a burned attempt costs a week.** The two that bind a first-time setup
are the **failed-validation limit** (a handful of failures per account per hostname per hour) and
the **duplicate-certificate limit** (five per week for the identical set of names). Neither is
recoverable by retrying harder; both are recoverable by waiting.

**Therefore: never retry issuance without changing something.** If `provision.sh`'s `tls` phase
fails, the sequence is diagnose → fix → *rehearse against staging* → re-run:

```sh
# The rehearsal the kit does not ship. --dry-run here is certbot's own flag: it exercises
# the entire HTTP-01 path against Let's Encrypt's STAGING server, saves nothing, and does
# not touch the production rate limits.
sudo certbot certonly --webroot -w /var/www/acme \
     -d bibitesmultiverse.com -d status.bibitesmultiverse.com \
     --dry-run --non-interactive --agree-tos -m <the ACME email>
```

Only when that succeeds, re-run `sudo provision.sh --only tls`.

**The four causes, in the order they occur:**

| Symptom | Cause | Fix |
|---|---|---|
| Challenge fetched from an unexpected IP, or a Cloudflare error page | The A record is proxied (T2) | Grey-cloud it, wait the TTL, re-verify with `dig` from off the box |
| Timeout connecting to port 80 | The Lightsail console firewall (T4), not `ufw` | Open 80 in the console; `ufw status` proves nothing about it |
| Validation succeeds against a different host | A stale AAAA record (T4) | Delete it, or point it at this instance |
| "does not resolve" | DNS has not propagated, or Phase 0 is incomplete | Wait. This is the one where waiting is correct |

If issuance is rate-limited anyway: **the map does not have to open on a schedule.** The instance
runs, `provision.sh --only tls` can be re-run when the window opens, and the rest of the box is
already built. Nothing is lost but time, and nothing has been announced yet.

### A mis-pointed A record

Caught in Phase 5 it costs a TTL. Caught in Phase 9 by the dry-run it costs nothing. Caught in
Phase 10 it costs an ACME attempt. Caught after issuance it costs a re-issue *and* — if a join
string has been minted — a message to every participant, because the advertised URL is baked into
every join string and D25's channel pushes nothing. **Treat the name and its record as permanent for
the run.**

### A firewall that blocks 80

nginx will be running and answering on the box (`curl -I http://127.0.0.1/` returns a 404 or a
redirect) while the outside world sees nothing. The distinguishing test is `nc -z -w5
bibitesmultiverse.com 80` from the development machine. Remember the *two* firewalls, and remember
that `provision.sh` only manages one of them.

### An instance too small

`monitor.sh`'s `replay` check is the tripwire and it fires in two stages. On **WARN**, plan the
resize — read `SIZING.md` §7's procedure, take a Lightsail snapshot, and pick an announced window.
On **CRIT**, the message is explicit: *the map is fine — the relay is unaffected — but the archive
must not be restarted until this is fixed.* An archive that will not come back from its replay is
the outage the whole tripwire exists to prevent, and `RESTART-POLICY.md` §2 makes "do not restart the
archive while replay is CRIT" the first line of the pre-restart checklist.

The fixes, in order, from `SIZING.md` §7: confirm the archive is a streaming build (it is, on this
box); then `GOMEMLIMIT`; then swap; then the retention rule; then a bigger instance. **On this
deployment the binding term is the resident set, and `GOMEMLIMIT` and swap do not fix a resident-set
problem** — the real answers are a bigger instance and a rule that bounds the *ledger*, which
`prune-genomes` deliberately does not do. So in practice: resize.

The resize itself is in `backup.sh --restore-help`: snapshot → new larger instance from the
snapshot → **attach the same static IP** → re-run `provision.sh`. Announce it as a case 4 restart.

### Disk

`monitor.sh` reports days-to-full from the box's own last 24 hours, which beats the model whenever
they disagree. WARN at 25% free, CRIT at 12%. The remedy is to grow the Lightsail volume online.
The reference failure is the 2026-08-08 ENOSPC outage on the living deployment, which stopped every
genome write and left durability damage that took days to understand — which is also why
`backup.sh`'s `auto` mode refuses to copy the ledger below 30% free rather than filling the volume
it is protecting.

### Anything that is not in a runbook

`m5_tracking.md`'s **stop rule** applies here as much as on the rig: an unfamiliar failure, a
deployment step no runbook describes, or a state that does not match what the documents record means
**stop, leave the system as found, and report with evidence.** Do not improvise a recovery on a
live deployment. This is doubly true once participants exist, because the evidence you would spend
is theirs.

---

## 7. What to record in the repository, as you go

**Agents do not commit.** Report changed paths and a proposed commit message; the orchestrator
stages explicit paths and commits. And documentation is **integrated, not appended**: read the whole
target document, merge into the entry that already covers the subject, surface contradictions rather
than stacking a second reading beside the first, and revalidate cross-references afterwards.

**`m5_tracking.md`, the WP3 row.** It is one long cell and it is rewritten rather than annotated.
What it needs from this work:

- The state moves from *"Remaining: the owner's 12 manual steps; then provisioning and the
  announcement"* to what actually happened, with dates: the account, the bundle bought, the instance
  and its region, the static IP being attached before the record, the certificate's issuance date
  and its names, the services enabled and proven across a reboot, the alert channel proven, the
  backup and its rehearsed restore, and the announced period's real dates.
- `MV_BUNDLE` stops being the open decision and becomes a recorded one.
- **The day-45 correction** described in §2. The row currently reads *"a 2 GB box holds the exit bar
  to ~day 45"*, which contradicts `SIZING.md` §4's day-28 resident crossing and `monitor.sh`'s own
  tripwire. Resolve it in favour of the kit.
- WP3's done-when has one clause this handoff does not close: *the forward receipt ships **with its
  measured per-migration cost at rate***. B26 is deployed on the rig; the measurement at a public
  map's rate belongs to WP8. Say so rather than letting the row imply it is done.

**`dev_environment.md`, once the service is live.** This document is the operational memory of the
machines, and the hosted instance becomes a second one. It has a *The living deployment* section
(and *The far end*, *The disk budget*, *Gotchas*); the hosted service wants its own section beside
them, holding what a future session needs and cannot re-derive: the instance's region and bundle,
its static IP, how SSH access is shaped, that `/etc/multiverse/deploy.env` is the one parameter file
and that it is 0640 root:multiverse, that the alert topic **exists** and where it is configured
(never the URL itself — it is a capability), the units and timers, and the paths for state, logs and
the kit's on-box copy. Any surprise you hit during provisioning belongs in *Gotchas* — including
anything in §4 that still catches you now that T9–T11 announce themselves, because that is exactly
the shape of thing the section exists for.

**What must never be recorded anywhere:** `MV_ALERT_URL`, any join string, any credential secret,
and the contents of `/var/lib/multiverse/joins/`. The first is a capability URL; the rest are other
people's identities.

**Contract and decision documents** need nothing from this work. `contract-b-m4.md` §23 B33–B34
already carries the retention amendment, and D24 is already in `system_decomposition.md`. The one
future exception is `WIND-DOWN.md` §4's note that D6's graduation call gets recorded in
`system_decomposition.md` at the close — that belongs to the ending, not to the opening.

---

## 8. What follows

### Publishing `v0.1.0`

The release is `release/README.md`'s four steps, done by hand, and it is gated on two things this
handoff produces or exposes:

1. **The announced period text** (Phase 14) fills `dist/RELEASE-PAGE.md`'s `@@OWNER:ANNOUNCED_PERIOD@@`
   marker — the build lists every `@@OWNER:…@@` marker it leaves.
2. **The repository being public.** `release/README.md` names this as a fifth step: the page's
   documentation links resolve only for somebody who can read the repository, so either the repo is
   made public or the four participant pages have to travel inside the release instead. Confirm
   which with the owner; it is a decision, not a detail.

The four steps are then: fill the owner-only fields, tag the commit the artifacts were built from
and push the tag, `gh release create` with the page as the body and all three `dist/` files as
assets, and read the published page as a stranger would.

**The ordering is a gate, not a formality:** D24's period must be stated before anybody joins, and
a join string handed out before the release exists points a stranger at software they cannot install.

### WP8 — the playtest

WP8 is the exit test: **at least four non-owner peers, at least 72 continuous hours, zero operator
actions on any participant's machine** (relay-side actions are allowed and counted). It is the first
time this map meets strangers, and Risk 2's accepted cost is that it **cannot be re-run cheaply** —
it spends other people's goodwill each time. Which is why the churn harness was already run to
exhaustion, the abuse cases rehearsed against the rig, and this handoff insists on a rebooted,
monitored, backed-up, restore-rehearsed service before the first join string leaves the box.

Two measurements WP8 owes that touch this work: the forward receipt's per-migration cost at a real
map's rate, and the two bands `docs/sidecar-diagnose-spec.md` left open. Both want the hosted map's
own numbers, not the rig's — the rig runs at eleven times a real map's rate and every figure from it
has to be divided before it means anything about strangers.

---

## 9. What this handoff does not cover

Stated rather than left to be discovered.

- **External reachability.** See §1. Test it from outside the instance after each front-door change.
- **Three defects this document found in the kit are no longer open, and this section no longer
  lists them.** The `deploy.env` ownership defect, the A-record check's loopback answer and the
  peers floor were all fixed in the kit after this handoff was written; **T9, T10 and T11 in §4
  carry what changed.** If you are reading a copy of this document that still lists them here, it
  predates the fix.
- **The live console's egress estimate predates its newest endpoints.** The published *boundary* is
  current in both `README.md` §5 and `nginx/multiverse-20-status.conf`, now stated as a rule —
  every handler on the archive's read-only mux is public, because nginx proxies `location /`
  wholesale — with a roster of sixteen beside it. But `SIZING.md` §6's ~32 GB/month per continuously
  open tab was derived before `/api/species/tree`, `/api/species/trends` and `/api/species/brains`
  existed, and it has not been re-derived. The landing page reads only `/api/status` every
  15 seconds. Recalculate the console figure against public traffic before transfer becomes a
  constraint.
- **DNS-01 is now possible and is not wired.** See T8.
- **The forward receipt's paragraph in `RESTART-POLICY.md` §1.2 is marked for revision.** It
  describes the world *without* B26; B26 has since shipped from a parallel arc. The paragraph says
  so and should be revised when the hosted relay runs it — which it will, from day one, since the
  binaries come from `HEAD`.
- **`m5_tracking.md`'s "12 manual steps"** is README §6's list, and this document's Phase order
  interleaves it with the script steps rather than replacing it. If they ever disagree, README §6 is
  the list the owner works from.
