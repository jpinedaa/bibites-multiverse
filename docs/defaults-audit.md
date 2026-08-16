# The public-safe defaults audit

**Every default this release ships with, what a bare install actually does with it, and a
verdict.** Decision 7 named four of them and asked for the audit before the software met
strangers. This audit was updated for release `0.2.4` on **2026-08-16**, against the code and
the package as they ship rather than against how they were described.

**Who this is for:** a reviewer, and the operator. A participant does not need to read it — the
part of it that affects a player is stated on the release page, in
[`participant/install.md`](participant/install.md) and in the installer's own output.

## What "a bare install" means here

Somebody downloads a package for their platform, runs its installer, and changes nothing else.
On Windows, that means the single-file setup, its included game, automatic public enrollment,
application shortcuts, and the selected start-after-install checkbox. On Linux, it means the
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

**Today.** The Windows setup installs `BibitesMultiverseLauncher.exe` and the sidecar below
`%LOCALAPPDATA%\Programs\Bibites Multiverse`. It creates one desktop shortcut and two Start Menu
shortcuts. The second Start Menu shortcut runs the uninstaller. The setup also creates one
per-user uninstall entry for Windows Settings.

**A bare Windows install.** Creates these entries. It does not add a service, scheduled task,
machine-wide registry value, or administrator requirement. The two launch shortcuts use the
project icon and open `BibitesMultiverseLauncher.exe`, the installed application. The generated
`Start-Multiverse.ps1` stays beside it for advanced and scripted use, and no shortcut points at
it.

**The cost.** The desktop receives one icon. Windows Settings receives one application entry.
The uninstaller removes both shortcut locations and the application entry.

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

1. Re-read the four defaults **in the source**, not in this document.
2. Re-run the guard tests: `nice -n 19 go test ./internal/relay -run 'InsecureNoToken|Servable'`.
3. Re-run `release/test-install-uninstall.ps1 -RealGameDir <Windows-game>`, whose suite fails on an installer that prints an
   execution-policy bypass or an `--insecure` instruction.
4. Run the finished Windows setup with `/PROBE` on Windows.
5. Re-measure the save footprint against the worlds the project is then running.
6. Grep `docs/` and `release/` for `--insecure` and check that every occurrence is still a
   prohibition.
