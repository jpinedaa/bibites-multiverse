# The public-safe defaults audit

**Every default this release ships with, what a bare install actually does with it, and a
verdict.** Decision 7 named four of them and asked for the audit before the software met
strangers. This audit was updated for release `0.3.8` on **2026-08-30**, against the code and
the package as they ship rather than against how they were described.

**Who this is for:** a reviewer, and the operator. A participant does not need to read it — the
part of it that affects a player is stated on the release page, in
[`participant/install.md`](participant/install.md) and in the installer's own output.

## What "a bare install" means here

Somebody downloads a package for their platform, runs its installer, and changes nothing else.
On Windows, that means the single-file setup, its included game, automatic public enrollment,
application shortcuts, and the selected start-and-open checkbox. On Linux, it means the
complete package, its included game, and automatic public enrollment. The mod settings below stay
identical on both platforms. Package and enrollment defaults are marked where they differ. Two
facts shape every verdict:

1. **The package ships no relay and no archive service.** Every participant artifact contains the
   installer, uninstaller, README, mod, sidecar, BepInEx, and support matrix. A complete artifact
   also contains an authorized game payload and its redistribution notice. Two defaults are not
   reachable from a participant install. They are audited because an operator can still use them
   on a host.
2. **The installer writes every setting explicitly, including the ones that match the mod's own
   default.** That is decision 7's second half: a future change to a default cannot silently move
   what an installed world does, and it cannot silently move what the project's own deployment
   measures either.

## The verdicts

| Default | Where it lives | A bare install | Verdict |
|---|---|---|---|
| `--insecure-no-token` | The relay, not the package | Cannot reach it: no relay is shipped | **PASS** — and the *"nothing enforces test-rig-only"* half is closed in code, by a bind refusal WP2 added after DQ4 was written |
| The archive's HTTP bind | The archive, not the package | Cannot reach it: no archive is shipped | **PASS at the packaging layer**, with a **finding for WP3**: the habit in the bring-up instructions, not the compiled default, is what would expose a public host |
| Public installer enrollment | Relay and all participant packages | Creates one unique credential over HTTPS and reuses it on retry | **PASS WITH FINITE LIMITS** — compiled off, explicitly enabled by the hosted deployment, bounded by nginx and relay limits, with verifiers stored instead of secrets |
| Windows application registration | The single-file setup | Creates per-user desktop and Start Menu shortcuts plus one uninstall entry | **PASS** — no administrator access, service, task, or machine-wide setting |
| `MULTIVERSE_MIGRATION_EXCLUDE` | The mod | Keeps the game's starter species home | **FIXED IN PACKAGING** — an empty value can no longer be reached by accident, and turning the policy off now takes a switch that says so and prints what it costs. One code-level finding remains, reported not fixed |
| `MULTIVERSE_SAVE_*` | The mod | A save every 10 minutes, 6 kept, save on quit | **PASS WITH A STATED COST**, and the cost is now measured rather than feared: 330–470 KB per save on the project's own worlds, so about 2.4–2.9 MB for the six kept and the live one |
| `MULTIVERSE_STARTUP_TIME_SCALE` | The mod | Every world starts at x10 instead of the game's x1 | **PASS** — it is a target the game itself governs down to protect the frame rate, it costs no disk and no network, and a player moves it with the speed slider they already have |
| `--inbound-admission` | The sidecar | `adaptive-shadow`: learns a machine budget at whatever speed the world runs and reports a population decision sized for the shared ×10 reference, but refuses nothing on population | **PASS FOR SHADOW ROLLOUT** — the speed slider changes nothing about the learner; no participant behavior changes until a later reviewed promotion to enforcing `adaptive`, and queue backpressure remains active |
| `--forward-timeout` | The sidecar | Records an unanswered forwarded organism lost after five wall-clock minutes | **PASS WITH A STATED LOSS** — the deadline bounds a full outbound journal. It does not send or return the organism again |
| The launcher's update check | The launcher | One anonymous background `GET` per launcher run to the project's own homepage, which can only add a line of text and a button | **PASS** — nothing waits on it, it carries no identity and no version, a failure is silent, and `MULTIVERSE_NO_UPDATE_CHECK` makes no request at all |
| `--keeper`, `--world-name` | The sidecar, set by the installers and the launcher | Publishes **neither**: an install nobody answered names nobody | **PASS** — the only default here is *unset*, the prompt shows the value before it is taken, and no account name is read off the machine when there is nobody to show it to |

A fifth flag belongs beside the first, and this audit adds it rather than leaving it implied:
**the sidecar's `--insecure-no-contract-a-token`**. It *is* shipped, inside the package.

---

## 1. `--insecure-no-token` — the relay's authentication switch

**Today.** The relay refuses to serve with an empty credential store
(`Server.CheckServable`, `go/internal/relay/relay.go`), and `--insecure-no-token` is the only way
past that. What DQ4 recorded as the open half — *"the flag is documented as test-rig-only and
nothing enforces that"* — **is no longer true**, and this audit is where that is written down:

- **The flag refuses any bind that is not loopback**, at startup, before a listener exists
  (`go/internal/relay/main.go`, and `contract-b-m4.md` §22 B23). The refusal names the remedy:
  bind `127.0.0.1` for a single-machine rehearsal, or mint credentials and drop the flag.
  `TestInsecureNoTokenRefusesANonLoopbackBind` pins it; re-run 2026-08-12, passing.
- **It logs one loud line per accepted connection**, naming itself as a test-rig-only flag and
  never for a map with peers on it.

So DQ4's *"make it impossible, or make it impossible to miss"* is answered by the first of the
two: a relay carrying this flag **cannot be reached from another machine**.

**A bare install.** Cannot reach it. The package contains no relay binary, so a player cannot
pass this flag whether or not they want to.

**The instruction half.** No file this project ships instructs anybody to pass it. Every
occurrence of `--insecure` in `docs/` and `release/` is a prohibition: the taxonomy's standing
rule, the release page's promise, the in-archive README's promise, and the installer's own
header. That is checked by the package's test — a run of the installer whose output contains an
execution-policy bypass or an `--insecure` instruction fails the suite.

**Verdict: PASS.** Nothing to fix at the packaging layer, and the code-level half is closed.

## 1b. `--insecure-no-contract-a-token` — the sidecar's, which *is* shipped

**Today.** The sidecar accepts a mod connection with no bearer token when this flag is set, and
logs one loud warning per accepted connection: *any local process can drive this world's
migrations and impersonate this sidecar to the mod*, and *no document this project ships may tell
a player to pass it* (`contract-a.md` §21, A47).

**The two flags are named differently on purpose**, so that no runbook can confuse one for the
other. `participant/install.md` states that there are exactly two and that neither will ever be
asked for.

**A bare install.** Does not pass it, on either platform. The start script — `Start-Multiverse.ps1`
or `start-multiverse.sh` — is generated by the installer and its sidecar arguments are fixed and
identical in both: `--listen`, `--relay`, `--peer-id`, `--data-dir`, `--credential-file`. **The
Windows launcher passes exactly the same five and nothing else.** It takes their values from the
world profile it reads, `profiles\<world>.json`, and that file has no field that could add a sixth
argument — nor does it hold the secret, which stays in `peer-secret.txt` and reaches the sidecar as
`--credential-file`. A player would have to edit the generated script, or start a sidecar by hand,
to add the flag.

**Verdict: PASS.** Its blast radius is one machine's loopback, its default is off, and it
announces itself when on.

## 2. The archive's HTTP bind

**Today.** The compiled default is `127.0.0.1:8796` (`go/internal/archive/main.go`, the `--http`
flag), which is the right default. The habit is elsewhere: the project's own bring-up passes
`ARCHIVE_HTTP=0.0.0.0:8796`, and the runbook says to, in five places.

**A bare install.** Cannot reach it. No archive is shipped to participants.

**What is actually exposed by the habit.** Seven handlers, all `GET`, all read-only, no
authentication and no rate limit: `/`, `/healthz`, `/api/status`, `/api/hops`, `/api/species`,
`/api/species/history`, `/api/history`. Nothing there mutates, and **nothing there is a secret** —
what the page shows is what the relay already broadcasts to every peer on the map, which
`participant/join.md` states before anybody joins.

**So the finding is not a leak. It is a hoster copying a LAN habit onto the public internet**, and
getting an unauthenticated, unrated, unproxied listener on a box whose disk the same process is
filling. On a LAN with six known worlds that is right. On a VPS it is a decision, and this audit's
job is to make sure it is taken rather than inherited.

**Verdict: PASS at the packaging layer. FINDING for WP3**, whose bring-up is the one that will
copy the habit: publish the status page deliberately — behind the same reverse proxy and TLS the
relay uses, with a rate limit — rather than by binding the archive's own listener to `0.0.0.0`.
The participant documentation already promises a page, so the answer is almost certainly *public,
but not like this*.

## 2b. Public installer enrollment

**Today.** The relay's compiled default is off. A host must set
`MULTIVERSE_PUBLIC_ENROLLMENT=1` and finite total, per-address, and time-window limits. The public
hosting example uses 256 automatic credentials and 8 new identities per network address in 24
hours. nginx adds an outer limit of two requests per minute with a burst of three.

**A bare participant install.** Each installer creates a random secret and UUID locally. It sends
them to the exact HTTPS enrollment path. The relay stores a salted verifier and never returns the
secret. An exact retry is idempotent. Each installer keeps a private pending record until it
stores the final credential and install record. A lost response therefore does not spend another
identity. Windows uses an account ACL. Linux uses mode `0600`.

**A private-map install.** Does not use this endpoint. It applies the join string that an operator
issued.

**The cost.** Open enrollment lets any internet client request an identity. The finite total is a
deliberate service-capacity control, and an abusive client can consume it. Disabling enrollment
stops new identities without revoking credentials or disconnecting existing worlds. The operator
must watch the credential count and decide when to raise or close the limit.

**Verdict: PASS WITH FINITE LIMITS.** No reusable secret ships in a participant artifact. The client creates
the only recoverable secret, HTTPS protects it in transit, and the relay stores only its verifier.

## 2c. Windows application registration

**Today.** The Windows setup installs `BibitesMultiverseLauncher.exe` — the launcher's window —
with `multiverse-launcher.exe`, which is the same launcher's commands, and the sidecar, below
`%LOCALAPPDATA%\Programs\Bibites Multiverse`. It creates one desktop shortcut and two Start Menu
shortcuts. The second Start Menu shortcut runs the uninstaller. The setup also creates one
per-user uninstall entry for Windows Settings.

**A bare Windows install.** Creates these entries. It does not add a service, scheduled task,
machine-wide registry value, or administrator requirement. The two launch shortcuts use the
project icon and open `BibitesMultiverseLauncher.exe`, the window. **Nothing has a shortcut to the
commands**: `multiverse-launcher.exe` is installed beside the window for a script to call and is
opened by no icon, and the window's own **Open** menu (*Open the commands window*) is what opens
it in a console. The generated
`Start-Multiverse.ps1` stays beside them for advanced and scripted use, and no shortcut points at
it either. The window requests `asInvoker` in its manifest and nothing above it.

**One more file, and the installer does not write it.** The window remembers where it was —
size, position, maximised, whether the details pane was open and where its dividers were left — in
`%APPDATA%\Bibites Multiverse\launcher-window.json`, written when it closes. It carries numbers
and two flags: no identity, no secret, no path, nothing about a world. It is **not** in the install
record, because the installer never wrote it, so **the uninstaller leaves it** — which is the same
rule the uninstall follows everywhere: it removes what its record owns and nothing else. Deleting
it by hand costs a person their window layout.

**The cost.** The desktop receives one icon. Windows Settings receives one application entry.
The uninstaller removes both shortcut locations and the application entry, and both executables:
each one is in the install record, by path and by hash, so a file a later hand changed is kept.
**`0.3.1` is the first release where that is true of the folder as well.** In `0.3.0` a complete
edition's uninstall kept the launcher profile describing the installation it had just removed —
the rule that decides which paths a profile may name protected the managed game copy from the very
run reclaiming it — so `profiles\` stayed, the application directory around it stayed, and Windows
Settings was left with an entry whose folder nothing removed.

**Verdict: PASS.** The setup creates the normal application surface that a player expects. All
changes are per-user and the uninstaller owns them.

## 3. `MULTIVERSE_MIGRATION_EXCLUDE` — the species that stay home

**Today.** The default is `Basic bibite`, the game's starter species
(`MigrationExclusion.DefaultNames`). The variable is read **on presence**, so an explicitly empty
value disables the policy rather than falling back to the default. D18 chose the default to keep
founder stock off the lanes; the concern DQ4 raised is that a stranger who empties the variable
floods a shared map with seed genomes, **and the census shows that as entirely normal while it
happens**.

**A bare install.** Keeps the starter species home. Both installers write
`MULTIVERSE_MIGRATION_EXCLUDE` = `Basic bibite` explicitly into the start script they generate, and
the Windows installer writes the same value into `profiles\default.json`. Every world the launcher
creates gets it as well, and the launcher applies the same refusal: an empty value takes
`--no-migration-exclusion`.

**What changed in the packaging, and it is a real fix rather than a note** (the flag names below
are the Windows spelling; the Linux kit has the same two as `--exclude-species` and
`--no-migration-exclusion`, with the same refusal):

- **An empty `-ExcludeSpecies` is refused**, with a sentence saying what it would have done:
  *"that is a real choice and it takes its own switch"*. The accident is unreachable.
- **Turning the policy off takes `-NoMigrationExclusion`**, a switch whose name states the
  intent.
- **When it is off, the installer says so in yellow**, in the block where it states every shipped
  setting: *your world will export starter stock onto a shared map, and the map's census shows
  that as entirely normal while it happens*.
- The value lands in one place the player can read and edit, with the mod's own variable name
  beside it.

**The mitigating fact worth recording**, because it decides how much more is needed: **the
exclusion list is published on the wire** (`CONFIG_UPDATE`, §19 A42) and appears on the map's
settings view. An operator can see which world has the policy off, without asking. The census
does not show it; the settings do.

**One code-level finding, reported and not fixed** (this audit does not change mod code):

> **The mod announces a disabled policy at the same level as everything else.** With the variable
> explicitly empty, `MigrationExclusion.Describe()` returns `<disabled>` and the startup summary
> logs `[M4-D18] config: migrationExcludeSpecies=<disabled>` through `LogInfo` — one line among a
> dozen configuration lines. A policy that is *off* is the one configuration state worth a `WARN`,
> because it is the state a supporter reading somebody else's log needs to notice without knowing
> to look for it. The remedy is one log level, in `MultiverseConfig.LogSummary`, and it belongs to
> whoever next touches the mod.

**Verdict: FIXED IN PACKAGING**, with that one finding open in the mod.

## 4. `MULTIVERSE_SAVE_MINUTES`, `_SAVE_KEEP`, `_SAVE_ON_QUIT`

**Today.** `SaveMinutes` 10, `SaveKeep` 6, `SaveOnQuit` true. **These are also what the project's
own deployment runs** — the rig's `2`/`4` override was dropped on 2026-08-10 — so the audit and
the measurements finally describe the same numbers.

**A bare install.** A save every ten minutes, six kept beside the live one, and a save when the
game closes.

**The disk cost, measured rather than feared.** On the project's own five worlds, a save file is
**330–470 KB** (measured 2026-08-12, worlds of 7 to 261 organisms; the size is dominated by world
state rather than by population). Six kept plus the live one is therefore about **2.4–2.9 MB per
world**. DQ4's *"a footprint on a disk the player did not budget"* is real in principle and small
in fact at this scale, and the honest statement is the measurement plus its scaling: it grows with
the world, not with the number of days.

**The other half of this default is not disk, and it is the half that matters.** The save interval
is also the **stall cadence** (Risk 3): a world save blocks the thread the heartbeat is composed
on, the 2-second stall budget is breached routinely on the project's own deployment, and a long
enough stall closes the local session with `A-4004`. The consequence is a session churn and a
short delivery pause — **arrivals are held in the journal, in order, for the whole silence** — and
never a lost organism. Saving *less* often makes each stall no shorter; it makes them rarer and
the history deeper per file.

**What the packaging does.** Writes all three explicitly, prints them in the installer's settings
block with what they spend, and repeats them in `participant/install.md`. `LOCAL-SAVESTALL` in the
taxonomy carries the remedy and names *nobody, then you* as the actor, which is the honest
division for a symptom that is usually a fact of life.

**Verdict: PASS WITH A STATED COST.**

## 5. `MULTIVERSE_STARTUP_TIME_SCALE`

**Today.** `10`, applied by the mod once per world load. It is new in this audit because the
default it replaces was never a setting at all: **the game resets every world it loads to x1 in
code** (`SimulationManager.Start` writes `targetTimeScale` and `engineTimeScale` back to 1 on a new
game, a loaded save and an autosave reload alike). There is no stored speed anywhere —
`targetTimeScale` is a `[NonSerialized]` static with no `PlayerPrefs` key and the world archive
does not carry one — so nothing an installer writes to disk could have changed it. A default speed
can only be applied from inside the running game.

**A bare install.** The world comes up and runs at **x10**: the same thing as dragging the in-game
speed slider to `X 10.00`, and nothing more. The installer and the launcher both write the value
explicitly, so it is visible and editable in `Start-Multiverse.ps1` / `start-multiverse.sh` beside
the other four.

**What it spends, and what it does not.** It spends **nothing on disk and nothing on the network**:
a faster world does not save more often (the save timer is wall-clock) and does not talk more (the
heartbeat is wall-clock too). What it spends is **CPU on the machine that runs it** — and that
spend is bounded by the game rather than by this setting:

- `TimeController.CheckMinFPS` is a servo, not a floor. It drives the *applied* speed toward
  whatever keeps the frame rate at `UserSettings.minimumFPS`, which ships at **15**. A machine that
  cannot hold x10 runs slower and stays smooth; it does not stutter.
- `Time.maximumDeltaTime` is pinned to `Time.fixedDeltaTime`, so one rendered frame advances at
  most one simulation tick and the achieved rate cannot exceed `fps/simTPS` whatever the target
  says.

So x10 is a **ceiling a participant's machine is allowed to approach**, not a load it is made to
carry. A weak machine ends up where it would have ended up anyway; a strong one is no longer left
ten times slower than the map around it for no reason anybody chose.

**Why it is not x1.** The map's own worlds run in this range, and an inbound migration is paced in
**simulated** minutes: a world left at x1 drains its arrival queue ten times slower than its
neighbours fill it, which reads on the page as a slot with a deepening backlog and no fault in it.
The project's own cloud fleet learned the same thing from the other end — a world that systemd
restarted after an OOM kill came back at the game's x1 and ran there for over an hour with the
largest population on the map and a queue it could not drain (slot-5, 2026-08-16).

**Changing it.** Three ways, in the order a participant will reach for them: the **speed slider in
the game**, which owns the speed for the rest of the session the moment it is touched — nothing
re-asserts this default while a world is loaded; `timescale <x>` on the command file for a scripted
world; and the variable itself in the start script for every future start. `off` there means the
game's own x1, on purpose. A value that is not a number in the game's own `0.1`–`100` range is
refused with a warning and leaves x1 in place, so a typo cannot pause or stall a world.

**Verdict: PASS.**

## 5a. `--inbound-admission` — population-aware receiving

**Today.** The sidecar supports `off`, a configured `fixed` living-population limit, an adaptive
limit, and `adaptive-shadow`. The adaptive estimate uses the median of one hour of
`population × achievedTimeScale` samples, a 0.90 safety margin, and a reference target speed.
That target is a pricing divisor, not a speed the world must request or reach: it defaults to ×10
and moves only through `--inbound-target-time-scale` or
`MULTIVERSE_INBOUND_TARGET_TIME_SCALE`. The estimate needs ten samples, is bounded to 10–200 by
default, and persists its current evidence in the world's data directory.

**A bare install.** Uses `adaptive-shadow` sized for ×10. The in-game speed slider changes what
the world runs at, and the learner keeps measuring the machine at that real speed; selecting ×5
neither stalls learning nor rescales the published limit, because the budget samples are speed
products and the divisor stays put. A paused or not-yet-measurable heartbeat changes no sample. Shadow calculates and publishes the exact decision that adaptive
mode would make, including whether the gate would be closed, but it never refuses an organism
because of that decision. The existing 64-entry inbound-journal ceiling remains an independent
safety control and can still return `OVERLOADED` before custody.

**Why shadow is the participant default.** Production history shows a strong population/speed
relationship, but the safe limit is a property of each machine and ecology rather than one
universal number.
Shadow mode collects that machine-local evidence across saves and restarts without turning an
estimate into participant-visible routing policy. Promotion to enforcing adaptive mode requires
the rollout gates in [`population-admission.md`](population-admission.md), including at least 24
hours of stable samples and a reviewed fixed fallback for each host class.

**Controlled production override.** On 2026-08-23 the operator explicitly authorized an early
evidence-gathering promotion for the five local and six hosted worlds under direct control. Those
eleven services set `MULTIVERSE_INBOUND_ADMISSION=adaptive`; the bare-install value above remains
unchanged. Learned state persisted across the promotion, and the initial verification found all
eleven enforcing with no capacity sheds or discarded journal bytes. The exception, monitoring,
fallbacks, and rollback criteria are recorded in the rollout runbook.

**What it spends.** One small atomic JSON state write per valid wall-clock minute of measured
running speed. It sends no additional network request: the estimate rides the heartbeat the mod
already sends and the peer stats already broadcast.

**Verdict: PASS FOR SHADOW ROLLOUT.** The release exposes the decision before it gives that
decision authority to refuse a migrant.

## 5b. `--forward-timeout` — unanswered forwarded organisms

**Today.** The default is five wall-clock minutes. `MULTIVERSE_FORWARD_TIMEOUT` or
`--forward-timeout` can override it. The sidecar commits an outbound entry as `sent` before it puts
the attempt on a live relay connection. The destination can hold the organism after that point.

A relay queue refusal moves the entry only when its proof matches the current relay session,
destination, and reroute count. A forwarding receipt for the same attempt cancels that proof. The
sidecar keeps the tried destinations and first refusal deadline in the journal. It continues on the
same axis and bounces once when it exhausts the bounded walk. A drain refusal has no attempt proof,
so it cannot consume a destination or move the deadline.

At the deadline, the sidecar changes the entry to a `lost` tombstone. It increments
`lostForwardTotal`. It does not send the organism again, and it does not return the organism to its
origin. A late acknowledgment is good news because it proves that exactly one organism arrived.

**A bare install.** The installer supplies no override, so the sidecar uses five minutes. A
controlled test filled all 64 outbound journal positions with unanswered `sent` entries. Contract A
then refused each new export with `JOURNAL_FULL`. At five minutes, all 64 entries became lost and
export intake reopened. A 24-hour override kept the same intake closed after five minutes.

**The cost.** A slow acknowledgment can arrive after the sidecar records the organism lost. The
organism does not duplicate because the sidecar sends nothing at the deadline. A longer timeout
increases the wait and can restore the longer export block that B42 removed.

**Verdict: PASS WITH A STATED LOSS.** Five minutes bounds the local export block. At-most-once
migration accepts a loss and forbids an automatic resend or return.

## 6. The launcher's update check

**Today.** On, and new in `0.3.2`. It is the only thing in a participant install that reaches the
network without being asked to, so it is audited even though nobody set it: an outbound request a
bare install makes is a default whatever it is called.

**A bare install.** Opening the launcher starts one `GET` to
`https://bibitesmultiverse.com/api/release` in the background, reads at most 64 KiB of it, and
gives up after six seconds. If the answer names a strictly newer, strictly numeric release, the
launcher draws one line saying so with a button that opens the project's homepage. Nothing else
happens: no download, no replacement, no world interrupted, and no second request — one lookup per
launcher run.

**What it discloses.** A bare `GET`: no query, no body, no cookie, no identity, and a `User-Agent`
of `bibites-multiverse-launcher` with **no version in it**. The host it asks is the project's own,
which serves the same number to the homepage, so no third party is told that this machine exists.
"How many people run which release" is deliberately not answerable from what is sent — the request
is not a report, and this audit's own standard is that a default must not turn a participant into
one.

**What it cannot do.** The endpoint answers a version string and only a version string; the address
the button opens is compiled into the program. An endpoint that could hand the launcher a URL would
be an endpoint that could aim it anywhere, so it is not one. A launcher never fetches or installs
anything itself.

**When it fails, it says nothing.** No route out, a captive portal, a proxy, a 500, a body that is
not JSON: every one leaves the line undrawn and prints no error. A world start does not touch this
code at all, so a machine with no network starts worlds exactly as it did before.

**Changing it.** `MULTIVERSE_NO_UPDATE_CHECK` set to any non-empty value makes no request at all,
and it wins over `MULTIVERSE_RELEASE_URL`, which moves the address for a test or a private
rehearsal.

**Verdict: PASS.** One anonymous request to the project's own host, per launcher run, whose entire
consequence is a line of text and a button — and an off switch that means off.

---

## 7. `--keeper` and `--world-name` — the two names a world publishes about a person

**Today.** Unset, and new in this release. They are the first two fields this system publishes that
name a *person* rather than a machine — the handle whoever runs a world chose, and the name they
gave the world (`contract-b-m4.md` §33, B49) — so the question this audit asks about them is not
what they cost but **who chose them**.

**A bare install.** Nothing. A world that was not given a name publishes none, and every reader
renders that as unknown. There is no fallback anywhere in the chain: not the Windows or Linux
account name, not the computer's name, not the save file's name, not the data directory, and not
the world's own identity on the map. A sidecar started without the flags does not send the fields
at all.

**What a person is shown before anything is published.** Both installers and the launcher offer a
value — the account name for the handle, *"<that name>'s world"* for the world — **on screen, in an
editable field or a prompt with the value in brackets**, above a sentence saying it is published
publicly. Enter accepts what is shown, typing replaces it, `-` publishes none, and the same three
answers work at every prompt this project has. That is the whole of the difference between a
default and an offer: a default is what happens when nobody chooses, and here nobody choosing means
nothing is published.

**What happens when nobody is there.** A silent install, a scripted run, output redirected to a log,
`-Unattended`, stdin that is not a terminal, the graphical setup's own hidden install: **not one of
them invents a name.** On a computer that has never answered they publish neither, and no account
name is read off the machine to fill the gap. On one that has, they keep exactly what it already
publishes — a decline included — because that is a person's answer and a run with nobody there is
not the moment to revisit it. The graphical setup answers both from its own boxes instead, because
it is the screen the person is looking at, and on an upgrade those boxes arrive holding what this
installation already publishes rather than the account name.

**Redirected output counts as nobody being there**, on both platforms and deliberately. A keyboard
on standard input is not the question; being able to SHOW somebody the value before taking it is.
So each installer requires a terminal on every stream its question uses: on Windows, standard input
and standard output, which is where PowerShell writes its own prompt; on Linux, standard error,
where the prompt goes, and standard output, where the four lines that say what these names *are* go
— and standard input beside them. `install.ps1 > install.log` and
`./install-bibites-multiverse.sh > install.log 2>&1`, run from a terminal, are therefore silent
runs. Asking anyway would put the question in a file nobody is reading, hang the install on it, and
take a blindly typed Enter as consent to publish the account name.

**What it discloses when it is set.** Exactly what was typed, to every peer on the map and every
subscriber the operator has authorised — the same audience as every other field in the stats block,
listed in `participant/join.md` before a participant joins. It is a caption and never an address:
nothing keys, routes, matches or deduplicates on either, and two worlds may honestly carry the same
name.

**Bounds, and who applies them.** At most 64 UTF-8 bytes and no control characters. The **authoring
sidecar** clips and strips, so no reader downstream has to; the installers and the launcher refuse
rather than clip, because a person who typed a name too long can be told and asked again, and a
name shortened on somebody's behalf is not the name they chose. **The graphical setup checks its
two boxes on the form**, before it starts the install at all: the install it starts has no keyboard
and its output goes to a log, so a refusal raised in there would arrive as *"Installation stopped"*
over a machine that had already taken its place on the map. The command-line installers check as
they settle the value: a name typed at their prompt is said out loud and asked for again, and a
name given as an option stops the run.

**Changing it, and taking it back.** `profile set NAME --keeper -` publishes none from that world's
next start, and the launcher's edit window clears the box for the same effect; on Linux, where
there is no launcher, the `KEEPER` and `WORLD_NAME` lines of `start-multiverse.sh` are the same
switch, and `--keeper -` on another install run is the other. An upgrade keeps what an installation
already published, a decline included, and does not ask again: each installer reads the live file
first — the launcher's profile on Windows, the start script on Linux — and its own install record
behind it, and an option named on the run still wins over both.

**Verdict: PASS.** The only shipped default is *unset*, the value is shown before it is taken, and
declining costs a participant nothing.

---

## What this audit does not cover

- **The relay's eight capacity limits.** They are knobs with published values and their own table
  (`contract-b-m4.md` §3.3); a map's operator sets them and every peer is told what they are.
- **`--min-contract-version`, left unset.** Unset means no minimum, which is the right default for
  a map whose participants have not been given a release to upgrade to yet. It is raised only
  *after* the release that satisfies it is published (D25), which makes it a deployment decision
  rather than a shipped default.
- **Anything a player configures deliberately.** The audit is about what happens when nobody
  chooses.

## Re-running it

This audit is a release artifact and goes stale with the release. For the next one:

1. Re-read the defaults **in the source**, not in this document.
2. Re-run the guard tests: `nice -n 19 go test ./internal/relay -run 'InsecureNoToken|Servable'`.
3. Re-run `release/test-install-uninstall.ps1 -RealGameDir <Windows-game>`, whose suite fails on an installer that prints an
   execution-policy bypass or an `--insecure` instruction.
4. Run the finished Windows setup with `/PROBE` on Windows.
5. Re-measure the save footprint against the worlds the project is then running.
6. Grep `docs/` and `release/` for `--insecure` and check that every occurrence is still a
   prohibition.
7. Re-read what the launcher sends to `/api/release` — the headers as well as the URL — and check
   that section 6's claim of an anonymous request with no version in it still holds:
   `nice -n 19 go test ./internal/launcher -run 'Lookup|CheckAddress'`.
8. Re-check that an install nobody answered publishes no name:
   `nice -n 19 go test ./internal/launcher -run 'PublicNames'`, and the profile and start script an
   unattended install writes in both installer suites — `keeper` and `worldName` empty, and no
   `--keeper` in the sidecar's fixed arguments. Re-check the other half with them: that an upgrade
   keeps a name AND a decline, from the live file and from the install record alone — scenario H in
   `release/test-install-uninstall.ps1`, scenario M in `release/test-install-uninstall.sh`, which
   also runs an install with its output redirected under a real terminal and proves it asked
   nothing.
