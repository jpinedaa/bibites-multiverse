# The crossing to `contract-b/4.0`

**The window that moves the living deployment from `contract-b/3.5` + `contract-a/2.3` + mod
`0.6.3` to `contract-b/4.0` + `contract-a/2.4` + mod `0.6.4`.** Six worlds, two machines, one
relay, one archive. Everything below is literal and in dependency order, with the observable
that gates each step and the answer to *what if it fails here*.

Nothing in this directory has been run against the live rig. Every command was rehearsed on
2026-08-11 against copies — a copy of `ring.json`, a scratch relay on port 18795, scratch data
dirs — and the rehearsal's evidence is quoted where it settles something. The rig itself was
untouched: it has been up throughout and `e2e/relay-data-m4-lan/ring.json` is byte-identical to
what it was on 2026-08-06.

---

## 1. What this window costs, and why each cost is what it is

| | |
|---|---|
| **Five local worlds down** | **~5–7 minutes**, once. The phase table in §9 accounts for every second of it. |
| **Slot 6 (the far end) down** | From the moment the new relay starts until **its own operator** applies the new bundle. This machine cannot shorten that and must not try (D9). The bound that matters is **24 hours**: after that, entries this side is holding for slot 6 bounce home by themselves (`holdTimeoutMs`, §9.3), which loses nothing but widens §9.3's accepted duplication case. |
| **The archive's permanent ledger gap** | **~0 crossings — if the archive is restarted INSIDE the outage, which is what this runbook does.** The archive is down while every sidecar is also down, so there are no crossings to miss. Restart it later, with the map live, and the same ~150 s absence costs roughly **1,600 crossings** that are never in the record. The last measured restart cost **1,940**. That is §5.1 working as designed — the traffic is untouched, only the record has the hole — and it is why the archive is restarted **exactly once** and why that once is here. |
| **A custody replay burst on the way back** | Measured peak `custodyDepth` 55 and `pacedDepth` 14, draining to base in about four minutes with nothing lost. On this map that rides on top of a resting `custodyDepth` of 24–58 at ×100, so read it as *falling*, not as *zero*. |
| **BepInEx log history** | Nothing, **if P1's first block is run**. `LogOutput.log` is truncated on launch. Skipping the archive step cost about 33 saves' worth of history on 2026-08-10 and that gap is permanent. |

### The one arithmetic that has moved since it was written down

`dev_environment.md` sizes the archive's replay at **~93 s**, measured 2026-08-10 against
**3.7 M records / 1.22 GB**. On 2026-08-11 the ledger is **6.24 M records / 2.0 GB**. At the
same rate that is **~150 s of replay** and ~160 s of archive process downtime. The bring-up's
own wait was 300 s — barely 2× — and the patch in this directory raises it to 600 s, following
the rule the script already states: *size it from the ledger, and raise it rather than believe
a timeout here.*

---

## 2. The stop rules for this window

- **Anything that does not match this document, stop.** Leave the system as found and report
  with evidence. Do not improvise a recovery on a live deployment.
- **`e2e/run-m4-lan.sh build` must not be used.** It builds `go build -o bin/` in place, and a
  running sidecar holds `bin/sidecar` open — the build fails with `ETXTBSY`. P0.3 builds to a
  scratch directory and P3 renames.
- **The scratch directory must be on `/mnt/wsl/data`**, not `/tmp`. `mv` is only atomic within
  one filesystem; `/tmp` is on the 98 GB root and `bin/` is on the 251 GB `/mnt/wsl/data`
  volume, so a `mv` between them is a copy plus an unlink and not the atomic replace the
  rolling-binary rule depends on.
- **Slot 6, the relay and the collector are not this session's to drive.** The far end is a
  person's; the relay restart in this window is the owner's act, taken deliberately.
- **Every build and test is `nice -n 19`.** Unniced load on this host reproduces the sidecar
  session storm — on record twice.
- **`--insecure-no-token` and `--insecure-no-contract-a-token` appear nowhere in this window.**
  They are not a rollback path and no document this project ships may instruct anyone to pass
  them.

---

## 3. The owner's own steps

Everything in this section is a decision or an act nobody else can make. The rest of the
document assumes these are settled.

**Before the window:**

1. **Choose the window.** Five worlds down ~5–7 minutes. Pick a moment when the second
   computer's operator is reachable, because slot 6 is dark from the relay restart until they
   act, and the useful bound on that is 24 hours (§1).
2. **Decide about `--min-contract-version`.** It is **unset** by default and this runbook
   leaves it unset. Setting it to `contract-b/4.0` would make the far end's stale sidecar fail
   with a clear version refusal instead of a TLS failure — but B25 is explicit that a floor is
   raised only *after* the release that satisfies it is published, and the far end has not
   applied anything yet. Leaving it unset is the recommendation; raising it is the owner's
   call and it is one flag on `start_relay`.
3. **Decide what happens to `~/.multiverse-token`.** Nothing reads it after this window. It is
   mode 600 outside the repo, so leaving it costs nothing and deleting it removes a retired
   secret. Not a decision this runbook should make.
4. **Decide whether the status page on `8796` matters this time.** The far end's operator has no
   other view of the map, and the rejoin is the moment they most want one. `ARCHIVE_HTTP=0.0.0.0:8796`
   binds it inside WSL — P5 does that — but the Windows firewall rule and the portproxy for
   `8796` **have no record of ever being run**; the reboot ritual verifies only `8795`. So the
   page is probably loopback-only from off this machine. The elevated commands are in
   `dev_environment.md`, *Owner steps → The status page on the LAN*, and they are the owner's.
   Test from Windows or from the second computer, **never with `curl` from WSL** — there is no
   hairpin route and the false negative reads exactly like a blocked port.

**During the window — the far-end half, which only the owner can do:**

4. **Rebuild the far-end bundle AFTER the local mod deploy** — `farend/make-farend-bundle.sh`,
   after `bibites-mod/deploy.sh`, so the DLL in the zip is the one this machine is running. It
   is safe against a live deployment: it builds and reads out of `bibites-mod/bin/Release/` and
   never writes into `BepInEx/plugins/`. It does write `bin/multiverse-sidecar.exe`, which is a
   Windows binary no local process holds open. **Known stale text**: its closing message still
   says to copy the bundle "with the token file (`~/.multiverse-token`)". That is wrong at
   `contract-b/4.0` — see item 5 — and the script needs that line corrected in a separate change.
5. **Carry THREE files to the second computer, not one.** By hand; nothing here may send them
   (D9).
   - `farend/dist/farend-bundle.zip` — the rebuilt bundle.
   - `e2e/tls-m4-lan/ca.crt` — the relay's certificate authority. **Not a secret.**
   - the SECRET half of slot 6's join string, as `peer-secret.txt`. **This one is a secret**,
     it is printed exactly once by P0.5, and it is also filed at
     `~/.multiverse/slot-6-handoff.secret`. Delete that file once the far end has it.
6. **The far end's operator must agree to trust the CA.** `setup-farend.ps1` imports it into
   *their own user* trust store, `Cert:\CurrentUser\Root` — no administrator rights, no other
   account affected, and the installer prints the command that removes it again. Windows may
   ask them to confirm; if the import fails for any reason the installer says so, installs
   everything else, and prints the exact manual command. **This is a real request to make of
   somebody else's machine and it is the owner's to make, not this rig's.**
7. **Commit the rebuilt bundle** in its own commit. It is a tracked binary artifact with its
   own rebuild rule.

**After the window:**

8. **`dev_environment.md`'s *The LAN token* section describes a mechanism that no longer
   exists.** So do the `--token-file` invocations under *Driving one component by hand*. They
   are correct until the window and wrong after it, which is why this runbook does not touch
   them — the document records what is running. Updating them is the first act after the
   crossing lands.
9. **`e2e/run-m3.sh`, `run-m3-lan.sh` and `run-m2.sh` are left alone.** They speak retired
   wires and are the record of runs that happened. `run-m3.sh` keeps `ensure_token` and
   `TOKEN_FILE` because it still needs them; the M4 layer stops calling them.

---

## 4. Phase P0 — before the window. **No downtime. Nothing here is irreversible.**

Every step in P0 can be run with the rig serving, and abandoning the crossing after any of them
leaves nothing to undo. Do them all before taking anything down.

### P0.1 — the rig is at the bar

```sh
cd ~/bibites-multiverse
e2e/crossing/rig-check.sh
```

**Gate:** exit 0, `AT THE BAR: 6/6 live and modConnected, 24/24 lanes peer_live`.
A non-zero `custodyDepth` or `pacedDepth` is reported as a note and is normal at ×100 — the
signal is a depth that never falls, which one sample cannot show. `heldDepth` and
`timeoutBounces` must be 0.

**If slot 6 is dark**, stop and find out why before spending a window: this crossing takes it
dark deliberately, and starting from an unexplained absence means never knowing which one you
are looking at afterwards.

### P0.2 — the checkout, and the rollback copies

```sh
cd ~/bibites-multiverse
git fetch
git status --short          # expect only e2e/crossing/, .gitignore and the two farend/ files

rm -rf "$HOME/.multiverse-rollback-bin"
cp -a bin "$HOME/.multiverse-rollback-bin"
cp "/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx/plugins/BibitesMultiverse.dll" \
   "$HOME/.multiverse-rollback-BibitesMultiverse-0.6.3.dll"

B="$HOME/.multiverse/backup-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$B" && cp -p e2e/relay-data-m4-lan/ring.json "$B/"
ls -la "$HOME/.multiverse-rollback-bin" "$B"
```

**Gate:** `$HOME/.multiverse-rollback-bin/{relay,sidecar,archive}` exist, the `0.6.3` DLL copy
exists, and `ring.json` is in the timestamped backup. These are the whole of the rollback story
for P2 and P3 and they cost seconds now.

`bin/` is gitignored, so a copy of it is not a repository act. The archive's data directory is
**not** backed up and does not need to be: the ledger is append-only and carries no wire
version, so this crossing neither reads it differently nor writes it differently. Backing up
8.9 GB belongs to WP3, not to this window.

### P0.3 — build the new binaries to scratch

```sh
mkdir -p /mnt/wsl/data/crossing-build
cd ~/bibites-multiverse/go
nice -n 19 gofmt -l . && nice -n 19 go vet ./...            # both must print nothing
nice -n 19 go test -p 1 ./...                               # the whole suite, serialised
CGO_ENABLED=0 nice -n 19 go build -o /mnt/wsl/data/crossing-build/ ./cmd/...
ls -l /mnt/wsl/data/crossing-build/
```

**Gate:** `go vet` and `gofmt` silent, the test suite green, and six binaries in
`/mnt/wsl/data/crossing-build/`. `/mnt/wsl/data/crossing-build` is on the **same filesystem**
as `bin/`, which is what makes P3's `mv` atomic.

**`nice -n 19` on the test run is not optional.** `go test -p 1 ./...` unniced is the measured
reproduction of the sidecar session storm.

### P0.4 — mint the TLS material

```sh
cd ~/bibites-multiverse
e2e/crossing/mint-tls.sh
```

**Gate:** the script prints `leaf verifies against the CA` and a SAN list reading
`IP Address:127.0.0.1, DNS:localhost, IP Address:192.168.1.227, IP Address:172.24.110.174`.

Those four names are the whole of the reachability story and each one is load-bearing:

- **`127.0.0.1`** — the five local sidecars and the archive. They dial **`wss://127.0.0.1`**,
  not `ws://`. With `--tls-cert/--tls-key` the relay wraps its *whole* listener in TLS, so no
  plaintext path survives on loopback. Rehearsed: a plain HTTP `GET` to a TLS-configured relay
  on `127.0.0.1` answers `400 Client sent an HTTP request to an HTTPS server`.
- **`192.168.1.227`** — the far end's `-RelayHost`. Its TLS client verifies the name it
  *dialled*, and that is the Windows LAN address, not the WSL address the portproxy forwards
  to.
- **`172.24.110.174`** — the WSL address. Nothing dials it by name; it is there so a debugging
  dial from Windows straight at the VM does not fail on the name.
- **`localhost`** — the same loopback under its name, for a hand dial.

Output lands in `e2e/tls-m4-lan/`, which is **gitignored**. `ca.key` must be kept: re-minting
the CA costs a fresh trust import on the second computer, which is an errand D9 forbids this
machine to run.

### P0.5 — mint the credentials against the EXISTING `ring.json`

```sh
cd ~/bibites-multiverse
ALLOW_LIVE_RELAY=1 e2e/crossing/mint-credentials.sh
```

`ALLOW_LIVE_RELAY=1` is required and is deliberate: the running **`contract-b/3.5`** relay
never opens `peers.json` and never re-writes `ring.json`, so minting beside it is additive.
Minting against a *serving* `contract-b/4.0` relay is a different thing and the script refuses
by default.

**What it does, and why it is slot-preserving.** `--reserve-slot <peerId>@<col>,<row>` returns
the *existing* reservation untouched when that `peerId` already holds a slot — the `@<col>,<row>`
suffix is not even consulted — and mints a credential only for an identity that has none. So
the six live claims, their numbers and their coordinates survive exactly. The peerIds are
**`slot-1` … `slot-6`**, which is `run-m4.sh`'s `peer_of()`; there is no `peer-lan-*` form on
this map.

The equivalent by hand, which is what the script runs:

```sh
# the five local peers — the join string names the URL they will dial
bin/relay --data-dir e2e/relay-data-m4-lan \
  --advertise-url wss://127.0.0.1:8795/contract-b/v4 \
  --reserve-slot slot-1@0,0 --reserve-slot slot-2@1,0 --reserve-slot slot-3@2,0 \
  --reserve-slot slot-4@0,1 --reserve-slot slot-5@1,1

# the far peer — the join string names the address the SECOND COMPUTER dials
bin/relay --data-dir e2e/relay-data-m4-lan \
  --advertise-url wss://192.168.1.227:8795/contract-b/v4 \
  --reserve-slot slot-6@2,1

# the archive, whose grant is subscribe and is disjoint from peer (B27)
bin/relay --data-dir e2e/relay-data-m4-lan \
  --advertise-url wss://127.0.0.1:8795/contract-b/v4 \
  --mint-credential archive-main --grant subscribe
```

**Do not run those by hand.** Each prints a secret **once**, on stdout, that the relay keeps
only a verifier for and can never print again. The script exists to catch them into `0600`
files instead of into a terminal scrollback.

**Gate, all four:**

1. `ring.json is BYTE-IDENTICAL (<sha256>)` — the script prints the digest before and after and
   compares them. Rehearsed 2026-08-11 against a copy: same sha256, unchanged mtime, six
   `this peer already holds a slot; left alone` lines.
2. The map line reads `3x2: 1=slot-1@0,0, 2=slot-2@1,0, 3=slot-3@2,0, 4=slot-4@0,1, 5=slot-5@1,1, 6=slot-6@2,1`.
3. Six files exist under `~/.multiverse/`, mode `600`:
   `peer-slot-1.secret` … `peer-slot-5.secret`, `archive-main.secret`. **No local peer's secret
   is ever printed.**
4. Slot 6's join string is printed in full, once, and is also at
   `~/.multiverse/slot-6-handoff.secret`. **Capture it now** — it is the owner's to carry.

```sh
ls -la ~/.multiverse/
python3 -c "import json;d=json.load(open('e2e/relay-data-m4-lan/peers.json'));print([(p['peerId'],p['grant']) for p in d['peers']])"
```

expects `[('archive-main','subscribe'), ('slot-1','peer'), … ('slot-6','peer')]` — seven
records, one `subscribe`, six `peer`. **Back `peers.json` up.** It is the third durable file,
beside `ring.json` and the archive's set.

**Abort answer for the whole of P0:** nothing to undo. `peers.json` and `e2e/tls-m4-lan/` are
files the running 3.5 relay does not read; `/mnt/wsl/data/crossing-build/` is off `bin/`; the
patch is not applied. Walk away and the rig has not noticed.

---

## 5. The window. **Downtime starts here.**

### P1 — archive the BepInEx logs, then take the local rig down

**Archive first, while the games are still up.** `cp` on a live log is safe; a launch truncates
it.

```sh
cd ~/bibites-multiverse
BEP="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx"
DEST="e2e/logs-m4-lan/bepinex"; mkdir -p "$DEST"
stamp="$(date +%Y%m%d-%H%M%S)"
for f in "$BEP/LogOutput.log" "$BEP"/LogOutput.log.[0-9]; do
  [ -f "$f" ] && cp -f "$f" "$DEST/$(basename "$f").pre-crossing.$stamp"
done
ls -la "$DEST" | tail -8
```

**Gate:** five `*.pre-crossing.<stamp>` files, non-empty, one per running instance.
That is exactly what the rig's own `archive_bepinex_logs` does; it is spelled out here because
there is no subcommand for it and because the one time it was skipped the loss was permanent.

Then take the whole local side down. `lan_down` stops the games, the five sidecars, the archive
and the relay, in that order, and archives the BepInEx logs again on its way out.

```sh
e2e/run-m4-lan.sh down
e2e/run-m4-lan.sh status
```

**Gate:** `status` reports `relay not running`, `archive not running` and
`sidecar-1..5 not running`, `no rig port is bound`, and `game.sh status` shows every recorded
instance gone. Confirm independently — `status` reads pid files, and a pid file is a claim:

```sh
ss -ltn | grep -E ':(8787|8788|8789|8790|8791|8795|8796) ' || echo "every rig port is free"
ps -eo pid,cmd | grep -E '/bibites-multiverse/bin/(relay|archive|sidecar)' | grep -v grep || echo "no rig process left"
```

The baseline collector keeps running through all of this and needs nothing. It is read-only —
it copies five save zips and curls `/api/status` every five minutes — so during the outage it
simply logs failures. Do not stop it and do not restart it; it is hand-launched and gitignored,
and the only thing that ever needs to relaunch it is a host reboot.

**Abort answer:** `ARCHIVE_HTTP=0.0.0.0:8796 ./e2e/run-m4-lan.sh up` brings the whole
`contract-b/3.5` rig back exactly as it was. `bin/` is untouched, the patch is unapplied, the
mod is unchanged. The cost is one archive replay (~150 s) and one custody burst.

### P2 — deploy the mod

```sh
bibites-mod/deploy.sh
```

**Gate:** `Deployed BibitesMultiverse.dll -> …/BepInEx/plugins`, and no
`WARNING: the game updated` line above it. A stale-`libs/` warning means Steam moved the game
under the rig — **stop**, because a game update is its own event with its own ritual and a
crossing is not the moment to discover it.

**Two things this deploy does NOT do, and both are worth knowing before the window:**

- **It adds no BepInEx config entry.** `0.6.4` binds no new configuration key — the two Contract
  A token variables are deliberately environment-only, because a secret in `config/` would be
  one file shared by every instance. So the first-bring-up config race
  (`configuration failed — the multiverse client stays off`) **cannot fire on this deploy**.
  That race is the one that cost two slots on the `0.6.1` rollout.
- **It does not have to be undone.** A `0.6.4` mod against an *old* `contract-a/2.3` sidecar
  works: the sidecar ignores the `Authorization` header as an unknown HTTP header (A52). The
  reverse is not true — a `0.6.3` mod against a new sidecar is refused with 401 on the ordinary
  ladder. **So a rollback rolls back the Go side and leaves the mod alone.**

**Abort answer:** restore `$HOME/.multiverse-rollback-BibitesMultiverse-0.6.3.dll` over
`BepInEx/plugins/BibitesMultiverse.dll` and bring the old rig up. Rarely necessary, per the
paragraph above.

### P3 — put the new binaries in place

```sh
cd ~/bibites-multiverse
mv /mnt/wsl/data/crossing-build/relay   bin/relay
mv /mnt/wsl/data/crossing-build/sidecar bin/sidecar
mv /mnt/wsl/data/crossing-build/archive bin/archive
mv /mnt/wsl/data/crossing-build/fakemod bin/fakemod
mv /mnt/wsl/data/crossing-build/ringstat bin/ringstat
mv /mnt/wsl/data/crossing-build/worldstat bin/worldstat
bin/relay --help 2>&1 | grep -E 'mint-credential|tls-cert|token-file' || true
```

**Gate:** `--mint-credential` and `--tls-cert` are listed and **`--token-file` is not**. That
one grep is the cheapest possible proof that the binary in `bin/` is the `contract-b/4.0` one.

### P4 — apply the new-version launch material

```sh
cd ~/bibites-multiverse
git apply --check -p1 e2e/crossing/contract-b-4.patch && git apply -p1 e2e/crossing/contract-b-4.patch
git diff --stat
bash -n e2e/run-m4.sh && bash -n e2e/run-m4-lan.sh && echo "both parse"
grep -c token-file e2e/run-m4.sh e2e/run-m4-lan.sh
```

**Gate:** `e2e/run-m4.sh | 134 +++…` and `e2e/run-m4-lan.sh | 47 +++…`, both files parse, and
`grep -c token-file` reports **0** for both.

**Why the patch is applied HERE and not earlier.** It is the first step *inside* the window
because an edited script plus the old binary is the failure the ordering exists to prevent: a
sidecar that crashed at 03:00 and was restarted through a patched `run-m4-lan.sh` against a
`contract-b/3.5` binary would be handed `--credential-file`, which that binary does not
understand. After P3 the binary in `bin/` is the new one, so the script and the binary agree
from this moment on and never disagree.

**Abort answer:** `git apply -R -p1 e2e/crossing/contract-b-4.patch` reverses it exactly;
restore `bin/` from `$HOME/.multiverse-rollback-bin/`; `up`. Still cheap.

### P5 — bring the rig back

One command, and it carries the whole regime — headless, ×100, `saveMinutes 10`, `saveKeep 6` —
because `run-m4-lan.sh` assigns all four unconditionally after it sources the rehearsal.

```sh
cd ~/bibites-multiverse
ARCHIVE_HTTP=0.0.0.0:8796 ./e2e/run-m4-lan.sh up
```

**`ARCHIVE_HTTP=0.0.0.0:8796` is not optional.** Without it the archive binds its compiled
`127.0.0.1:8796` and the second computer's operator loses their only view of the map — at the
exact moment they need it to see whether their rejoin worked.

`up` runs the following, and each has its own observable. Watch them go past; do not walk away.

| Step inside `up` | The observable |
|---|---|
| `ensure_credentials` | silent. It fails loudly and early if any of the TLS files or the six secrets are missing — which is the whole reason P0.4 and P0.5 come before any downtime. |
| `check_slot_ports` | `every Contract A port 8787..8792 is free on both sides of the WSL boundary`. A failure here means a Windows listener — check for a `0.0.0.0:8790` portproxy row, which is a regression, not a leftover. |
| map shape | `the map is already pre-placed: …`. **It must say this.** If it instead runs `reserve`, the map is not 3×2 and something is wrong with `ring.json` — stop. |
| `start_relay` + `wait_healthy` | `relay: terminating TLS at its own front door` then `relay: listening … scheme=wss path=/contract-b/v4 … credentials=7`. **`credentials=7` is the count that proves P0.5 landed.** |
| `start_archive` | the ledger-size note, then **~150 s of nothing**, then `archive: subscribed to the relay`. The wait is 600 s in the patched script. **The status page answers nothing during the replay — that is not a fault.** |
| five sidecars, one at a time | per slot: `/healthz`, then `contract B: slot granted … slot=N position={Col:… Row:…} reason=reclaimed`. **`reason=reclaimed` and the peer's own coordinate, per slot.** |
| five games, one at a time | `[M2-WORLD] world 'M4-SlotN' loaded`, then `[M2] CONFIG_UPDATE reason=connect` for each. |
| the local lanes | every slot's east lane open, and every slot's north except slot 3's. Slot 3's north is deliberately not waited on: column 2 is {3, 6} and it closes with `no_peer` while the far end is away. |
| the time scale | `send <n> timescale 100` on all five. |

**Two failure modes to recognise while this runs**, both on record and both with a remedy that
is *one instance*, not the rig:

- **A slot comes up `live` with `modConnected` false and its BepInEx log has
  `configuration failed`** — the config race. It cannot happen on this deploy (P2), but check
  anyway: `grep -a 'configuration failed' "$BEP/LogOutput.log"*`.
- **A slot comes up `modConnected` false with *nothing at all* in any log** — the log-file
  starvation trap. BepInEx hands out five log files and then gives up; an instance with no log
  file never loads the mod. The tell is the *absence*. The remedy is to restart **that one
  instance** through the rig's own `start_game`, so its environment matches the other four
  exactly — which matters more than ever now, because that environment is where
  `MULTIVERSE_CONTRACT_A_TOKEN_FILE` comes from.

### P6 — the gates `up` does not check

**P6.1 — zero discarded journal bytes, per slot.** This is the rolling-restart gate and `up`
does not assert it.

```sh
grep -c discardedBytes e2e/logs-m4-lan/sidecar-*.log
```

**Gate: `0` for all five.** The sidecar logs `discardedBytes` at **error** level and *only*
when a journal was damaged and replay stopped early — so any count above zero means custody
history D2 promised is durable has been lost, and only a person can judge what it held.
**Stop and report; do not restart anything.**

**P6.2 — every process is on the new binary.**

```sh
cd ~/bibites-multiverse
for f in e2e/run-m4-lan/m3-*.pid; do
  p="$(cat "$f")"; n="$(basename "$f" .pid)"; n="${n#m3-}"
  b="bin/${n%%-*}"
  printf '%-12s pid %-8s %-9s %s\n' "$n" "$p" \
    "$( [ "$(sha256sum /proc/$p/exe 2>/dev/null | cut -d' ' -f1)" = "$(sha256sum "$b" | cut -d' ' -f1)" ] && echo SAME || echo DIFFERENT )" \
    "$(readlink /proc/$p/exe 2>/dev/null)"
done
```

**Gate:** every line reads `SAME` and no `readlink` ends in `(deleted)`.

**This is not the check `dev_environment.md` describes, and the recorded one does not work on
this host.** It says to compare `/proc/<pid>/exe`'s inode against `bin/sidecar`'s. Measured
2026-08-11: `stat -c %i /proc/<pid>/exe` here returns the **procfs entry's own** inode (a
distinct `7xxxxxx` value for every pid), not the target file's, so the comparison can never
match and says nothing. The two things that do work are the content digest read *through* the
magic link, and the `(deleted)` suffix `readlink` prints when a process is holding an inode
that has since been replaced. Both are in the loop above.

**P6.3 — the time scale actually took.** The first `timescale` after a world load answers
`Time.timeScale=1.00` and the reported scale sticks there; a second send twenty seconds later
takes.

```sh
sleep 30
for s in 1 2 3 4 5; do e2e/run-m4-lan.sh send "$s" timescale 100; done
sleep 30
e2e/crossing/rig-check.sh --expect 5
```

**Gate:** each send answers `targetTimeScale=100.00 Time.timeScale=100.00`, and `rig-check`
reports all five local slots at `timeScale 100` with `10/6`. Read readiness off `/api/status`,
never off a log line mark — a restarted game can land on a *different* BepInEx log file than
the one it left, so a mark taken before the quit is meaningless and a `wait_log` against it
hangs.

**P6.4 — the map with the far end still away.**

```sh
e2e/crossing/rig-check.sh --expect 5
```

**Gate:** 5/5 live and `modConnected`, 20 lanes declared, `heldDepth` and `timeoutBounces` 0,
and two lanes closed with `no_peer` (slot 3's north and south) reported as a note. Slot 5's east
names slot 4 — the bypass around the hole at (2,1). **That is the correct picture of a map
waiting for its sixth world**, and it is what the far end's arrival will change.

---

## 6. The far-end half

Nothing in this section runs on this machine. Steps 1–3 are the owner's; step 4 is the second
computer's operator's; step 5 is the only observable this side gets.

1. **Rebuild the bundle** — `farend/make-farend-bundle.sh` — **after** P2's `deploy.sh`, so the
   DLL in the zip is byte-identical to the one deployed here. **Gate:** the script prints
   `setup-farend.ps1 pins the same hash` and lists the zip's contents.
2. **Commit the rebuilt bundle**, in its own commit.
3. **Hand over three files** — the bundle, `e2e/tls-m4-lan/ca.crt` as `ca.crt`, and slot 6's
   secret as `peer-secret.txt` — plus the relay host, `192.168.1.227`.
4. **On the second computer**, in PowerShell in the unpacked folder:

   ```powershell
   Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
   Get-ChildItem . | Unblock-File
   .\setup-farend.ps1 -RelayHost 192.168.1.227 -CaFile .\ca.crt `
       -PeerSecretFile .\peer-secret.txt
   .\start-slot6.ps1
   ```

   The installer now runs **seven** steps rather than six: the game, the version pin, BepInEx,
   the plugin, **the CA import**, **the credential**, and the generated scripts. It prints the
   CA's subject, thumbprint and expiry before importing, states plainly what trusting it means,
   reads the store back by thumbprint to prove the import landed, and — if it did not — installs
   everything else and prints the one command that finishes the job. `-SkipCaImport` is there
   for an operator who would rather do it themselves.

   `start-slot6.ps1` starts the sidecar first and waits for `contract B: slot granted` before
   starting the game. That order was already right and now has a second reason: **the sidecar
   mints the Contract A token file the game needs**, so a game that came back first would meet
   an enforcing sidecar and get a 401 on the ordinary ladder.

5. **The observable, from here.** `relay.log` records the far sidecar leaving and a new one
   connecting and reclaiming its slot:

   ```sh
   grep 'peer=slot-6' e2e/logs-m4-lan/relay.log | grep 'client '
   ```

   **Gate:** a `client gone` / `client connected` pair, then a placement claim with
   `reason=reclaimed` for slot 6. A mod-only bounce cannot produce that triple — it logs
   placement claims only — so the triple is the signature of the installer having run and
   replaced `multiverse-sidecar.exe`.

   Then:

   ```sh
   e2e/crossing/rig-check.sh --wire
   ```

**What cannot be determined from here, and must not be guessed at:** whether the far end has
applied anything, until the triple appears. Until it does, slot 6 is dark, the map runs 5/6 with
a hole at (2,1), and that is the designed state and not a fault.

**The bound on waiting is 24 hours**, and it is `holdTimeoutMs`. Entries this side has taken
custody of and addressed to slot 6 are held while it is dark; after the accrued 24 hours they
bounce home by themselves. Nothing is lost either way — a bounce widens §9.3's accepted
duplication case, and that is the whole cost.

---

## 7. Post-window verification

```sh
e2e/crossing/rig-check.sh --wire
```

**The full bar, all of it:**

| What | Where it is read | Expected |
|---|---|---|
| 6/6 slots live and `modConnected` | `/api/status` | six, no holes, no unknown |
| 24/24 lanes | `/api/status`, `sum(len(exportEdges))` over live slots | 24 open, 0 closed, 0 bypass, every one `peer_live` |
| the archive's own dial | `relayUrl` on `/api/status` | `wss://127.0.0.1:8795/contract-b/v4` |
| Contract A's version | `contractAVersion` per slot | `contract-a/2.4` on all six |
| the mod's version | `modVersion` per slot | `0.6.4` on the five local slots; slot 6 also `0.6.4` once it applies the bundle |
| Contract B's version | `relay.log` — there is **no** contract-b field on the status page | `relay: listening … scheme=wss path=/contract-b/v4` and `credentials=7` |
| the regime | `/api/status` | five local slots `timeScale 100`, `saveMinutes 10`, `saveKeep 6` |
| queues | `/api/status` | `heldDepth` 0, `timeoutBounces` 0; `custodyDepth` and `pacedDepth` **falling** |
| journals | `grep -c discardedBytes e2e/logs-m4-lan/sidecar-*.log` | `0` on all five |
| errors | `e2e/run-m4-lan.sh errors` | no unexplained error line |
| the ledger | `tail -c 4096 e2e/archive-data-m4-lan/migrations.jsonl \| tail -2`, and `ledgerRecords` climbing between two `rig-check.sh` runs | crossings recorded again, with timestamps after the window. **Do not run `bin/archive list` for this** — it walks all 6.24 M records and prints every one |

**The one thing to check that no gate above covers:** that Contract A is actually
*authenticated* rather than merely working. The sidecar mints its token at first start and the
mod presents it on every dial:

```sh
cd ~/bibites-multiverse
BEP="/mnt/c/Program Files (x86)/Steam/steamapps/common/The Bibites/BepInEx"
ls -la e2e/data-m4-lan/slot-*/contract-a.token       # five files, mode -rw-------
grep -ha 'contract A token' "$BEP/LogOutput.log"* | sed 's/\r$//' | sort -u
```

**Gate:** five `0600` token files, and one `[M2] contract A token:
MULTIVERSE_CONTRACT_A_TOKEN_FILE=\\wsl.localhost\Ubuntu\mnt\wsl\data\…\slot-N\contract-a.token`
line per instance. The **Windows** form of the path is the point: `WSLENV`'s `/p` flag
translated it on the way out. If a line reads `<unset>`, the variable did not cross the
boundary and that slot's mod is dialling bare — it will still be refused with 401 and reconnect
on the ladder, and the fix is to restart that one instance through `start_game`.

Nothing anywhere writes the token's *value*. Only the path.

---

## 8. Abort and rollback, by stage

| After… | Recoverable? | How |
|---|---|---|
| **P0 (all of it)** | **Completely.** | Nothing to undo. `peers.json` and `e2e/tls-m4-lan/` are inert to the 3.5 relay; the build is off `bin/`; the patch is unapplied. |
| **P1 (games and rig down)** | **Completely.** | `ARCHIVE_HTTP=0.0.0.0:8796 ./e2e/run-m4-lan.sh up`. Costs one archive replay and one custody burst. |
| **P2 (mod deployed)** | **Completely, and usually unnecessary.** | `0.6.4` works against a `2.3` sidecar (A52), so a Go-side rollback does not need a mod rollback. If it is wanted: copy `$HOME/.multiverse-rollback-BibitesMultiverse-0.6.3.dll` back into `BepInEx/plugins/`. |
| **P3 (binaries replaced)** | **Completely.** | `cp $HOME/.multiverse-rollback-bin/* bin/`, then `up`. |
| **P4 (patch applied)** | **Completely.** | `git apply -R -p1 e2e/crossing/contract-b-4.patch`, restore `bin/`, `up`. |
| **P5, before the archive has re-subscribed** | **Completely.** | Same as P4. The archive has not written a byte yet. |
| **P5, after the archive has re-subscribed** | **Yes, but the return trip is no longer free.** | Rolling back now costs a **second** archive restart. Do it while the sidecars are still down and it costs ~0 ledger records; do it after the map is live and it costs ~1,600 crossings, permanently. **This is the point at which "abort" stops being cheap.** |
| **P6 / a single slot failing its gate** | **Per slot, and the map absorbs it.** | Stop that one sidecar. Its position becomes a hole, its neighbours' lanes re-pair around it, and the other four keep running. Its journal is on disk and its reservation never expires. Do not restart the relay to fix one slot. |
| **The far end** | **Not this machine's to roll back, ever.** | It stays on the old bundle until its operator acts. Its journal, its world and its slot survive indefinitely. |

### What is not recoverable, stated plainly

1. **A ledger gap.** Any archive absence while the map is live is crossings that are never in
   the record. §5.1 by design. The only lever is *when* the restart happens.
2. **BepInEx log history**, if P1's archive step is skipped. Truncated on launch, gone.
3. **A join string.** The relay keeps a verifier. The only recovery is
   `--handover-slot <n>=<newPeerId>`, which mints for a **new** identity and drops the old — and
   for slot 6 that means the second computer has persisted a `peerId` that no longer holds a
   slot, so it is an errand on somebody else's machine.
4. **The CA private key.** Lose `e2e/tls-m4-lan/ca.key` and a renewed certificate means a new
   CA, which means a fresh trust import on the second computer.

---

## 9. Estimated downtime, per phase

Measured values are marked; the rest are derived from a measurement and say from what.

| Phase | What is down | Estimate | Basis |
|---|---|---|---|
| P0 | nothing | 15–30 min of operator time, **zero downtime** | new work; `go test` dominates |
| P1 archive logs | nothing | ~5 s | a `cp` of five files |
| P1 `down` | five worlds, five sidecars, archive, relay; slot 6 loses its session | ~15 s | `lan_down` sleeps 2 + 2 plus the quits |
| P2 mod deploy | as above | 20–40 s | `dotnet build -c Release` warm, plus one file copy |
| P3 `mv` binaries | as above | ~2 s | six renames within one filesystem |
| P4 apply the patch | as above | ~5 s | `git apply` plus two `bash -n` |
| P5 relay up | as above | ~5 s | `wait_healthy` on `https://127.0.0.1:8795/healthz` |
| **P5 archive replay** | **as above — this is the single largest term** | **~150 s** | 93 s measured 2026-08-10 at 1.22 GB / 3.7 M records, scaled to today's 2.0 GB / 6.24 M |
| P5 five sidecars | five worlds still down | 25–40 s | 5 × (start + `/healthz` + grant), ~5–8 s each |
| P5 five games | five worlds | 60–120 s | `up` sleeps 8 s between starts, then waits for `[M2-WORLD]` and `CONFIG_UPDATE` per instance |
| **Total, map down** | | **~5–7 minutes** | sum of the above; the recorded mod-deploy-only budget is 100–110 s and the archive replay is what makes this window four times that |
| P6 settle | nothing — the map is live | ~60 s of checks, then ~4 min of custody drain | measured burst: `custodyDepth` 55, `pacedDepth` 14, draining to base in ~4 min with nothing lost |
| Far end | slot 6 only | **unbounded, and not this machine's to bound** | a person on another computer; the useful limit is `holdTimeoutMs` = 24 h |

---

## 10. What is in this directory

| File | What it is | How it is applied |
|---|---|---|
| `RUNBOOK.md` | this document | read |
| `contract-b-4.patch` | **a ready-to-apply unified diff** against `e2e/run-m4.sh` and `e2e/run-m4-lan.sh` — not new copies of them. 161 insertions, 20 deletions, two files. Verified with `git apply --check` and with `bash -n` on the result | `git apply -p1 e2e/crossing/contract-b-4.patch`, at **P4** and not before |
| `mint-tls.sh` | mints the CA and the relay's server certificate with the four SANs the rig actually dials, into the gitignored `e2e/tls-m4-lan/` | P0.4 |
| `mint-credentials.sh` | mints the seven credentials against the **existing** `ring.json` and captures each secret into a `0600` file; prints only the far end's | P0.5 |
| `rig-check.sh` | read-only: reads `/api/status` and says whether the map is at the bar | P0.1, P6.3, P6.4, §7 |

### Everything the patch changes, and why

| File | Line | Change |
|---|---|---|
| `run-m4.sh` | `RELAY_URL` | `ws://…/contract-b/v3` → **`wss://…/contract-b/v4`**, plus a new `RELAY_HEALTH` on `https://` |
| `run-m4.sh` | new block | `TLS_DIR` / `TLS_CERT` / `TLS_KEY` / `TLS_CA`, and **`export SSL_CERT_FILE`** |
| `run-m4.sh` | new block | `SECRETS_DIR`, `ARCHIVE_CREDENTIAL`, `cred_of()` |
| `run-m4.sh` | new function | `ensure_credentials()` — **retires `ensure_token`**. It mints nothing; it refuses to start a rig whose TLS files or six secrets are missing |
| `run-m4.sh` | `start_relay` | `--token-file` **removed**; `--tls-cert`/`--tls-key` added; optional `--advertise-url` |
| `run-m4.sh` | `start_archive` | `--token-file` → **`--credential-file "$ARCHIVE_CREDENTIAL"`** (the subscribe grant) |
| `run-m4.sh` | `start_sidecar` | `--token-file` → **`--credential-file "$(cred_of "$slot")"`** |
| `run-m4.sh` | `start_fakemod` | **`--contract-a-token-file`** added — the rehearsal's synthetic peer is a mod for A47's purposes |
| `run-m4.sh` | `start_game` | **`MULTIVERSE_CONTRACT_A_TOKEN_FILE`** set, and named in `WSLENV` with the **`/p`** flag |
| `run-m4.sh` | `reserve` | delegates to `mint-credentials.sh` instead of piping six unrecoverable join strings through `sed` |
| `run-m4.sh` | `up` | `ensure_token` → `ensure_credentials`; `wait_healthy` on `$RELAY_HEALTH` |
| `run-m4-lan.sh` | new block | `LAN_RELAY_HOST`, `RELAY_ADVERTISE_URL`, `FAR_PEER` |
| `run-m4-lan.sh` | `far_end_hint` | the new `setup-farend.ps1` invocation, and the three files the far end needs |
| `run-m4-lan.sh` | `reserve` | as `run-m4.sh`, with the far peer named |
| `run-m4-lan.sh` | `lan_up` | `ensure_credentials`; `$RELAY_HEALTH`; the far-end note now names `wss://` and the CA; the archive replay wait **300 s → 600 s** |

`run-m3.sh`, `run-m3-lan.sh` and `run-m2.sh` are **not** in the patch. They speak retired wires
and keep their own token plumbing.
