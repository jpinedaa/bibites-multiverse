# M5 Considerations — Public Release

This report expands milestone M5 of `system_decomposition.md`.

**Status: PROPOSED. Nothing in this document is ratified.** M4 is complete and its rig runs
on as the living deployment. This is the design pass that opens M5, written before any work
starts and before the owner has signed anything. Two sections hold what needs a signature:
*Scope the documents do not state*, which is new scope nobody has agreed to yet, and
*Decisions for the Owner*, which is nine calls this document recommends and cannot make.
Every recommendation below is a recommendation. The owner overrode three of them in M4 and
three more in M3, and this document is written to be overridden the same way.

The milestone was created by D9, renumbered by D16, and its scope has not changed since:
`system_decomposition.md`, *M5 — Public release*. The starting point is M4's join kit
(`farend/`) and the wire as it stands — `contract-a/2.3` and `contract-b/3.5`, which is what
the contracts said on 2026-08-07 and what the living deployment speaks today. Read the
contracts for the current pair rather than this line.

## Purpose

M4 made the ring survivable. M5 makes it public, and that is a much smaller change to the
code than it is to the assumptions.

Every property M4 proved was proved against machines the owner owns. Six worlds, two
computers, one shared token, one build compiled the same afternoon, and one operator who can
walk to the second machine when it needs something. Four assumptions are wired into that
picture, and M5 removes all four at once:

1. **Every peer is trusted.** `contract-b-m4.md` §3.1 says so plainly: the token keeps an
   unrelated device off the LAN and authenticates no identity at all, so a token holder can
   present any `peerId` and evict the peer that legitimately holds it.
2. **Every peer runs the build the owner just made.** The rig upgrades in one step — three
   binaries and one mod, all built here. `contract-a.md` §15 A23 calls that out as the whole
   reason D13's breaking change landed in M4.
3. **Every peer's operator can be reached.** `dev_environment.md`, *The far end*, says the
   quiet part: whether the second computer applied a fix "cannot be determined from here, and
   no observation will settle it" — so you ask its operator.
4. **Every peer comes back.** A slot reservation never expires (D8), a genome fetch retries
   on a ladder that runs to daily, and a held entry waits 24 hours for a world that is
   assumed to be asleep rather than gone.

Each assumption is load-bearing somewhere. Most of the work below is finding where, and
deciding whether the answer is a mechanism, a published rule, or an accepted cost stated out
loud. M4's exit test was a script. M5's is a handful of people, which is the other thing that
changes: for the first time the milestone cannot be closed by the rig alone.

## The deployment after M5

```
   Today. Everything on the owner's own LAN, brought up by hand.

        main machine (WSL)                     second computer
        ┌────────────────────────────┐         ┌──────────────┐
        │ relay :8795  archive :8796 │◄──LAN───┤ sidecar 6    │
        │ sidecars 1-5, games 1-5    │         │ game 6       │
        └────────────────────────────┘         └──────────────┘
        plain HTTP · one shared token · one operator · one build

   After M5. The relay is somewhere else, and most peers are strangers.

                      ┌──────────────────────────────┐
                      │  VPS: relay + archive        │
                      │  TLS, per-peer credentials   │
                      │  capacity and abuse limits   │
                      │  supervised, monitored, DNS  │
                      └──────────────────────────────┘
                         ▲       ▲        ▲         ▲
                   ┌─────┘       │        │         └─────┐
             owner's rig     stranger  stranger      stranger
             (5 worlds,          A         B         who leaves
              still a peer)                          and never returns
```

The owner's rig does not stop existing. It becomes one participant among several, and it
keeps every operational habit `dev_environment.md` records — the reboot ritual, the
five-instance BepInEx ceiling, the manual collector relaunch. What it loses is its privileged
position: after M5, a fix applied here is not a fix applied to the map.

## Scope

In scope:

- A relay and an archive hosted on a VPS, operated as a service rather than started by hand
- TLS on Contract B, and per-peer credentials replacing the shared token — the two together
  (`contract-b-m4.md` §13 item 1)
- A bearer token on Contract A (`contract-a.md` §12 item 1)
- A subscriber authorisation rule for the archive (§13 item 4)
- An authenticated admin path for slot release, handover and eviction (§13 item 5)
- Capacity and abuse limits: per-peer connection, message and genome-request ceilings,
  payload quotas, and a policy for slot space that only ever grows
- The renderer's escaping obligation for attacker-chosen text, and the prohibition on parsing
  `contractAVersion` into a capability decision (§13 item 7)
- A relay forward receipt (§13 item 6), promoted from parked — Design Question 2 argues it
- Placement, auto-placement and route-around under continuous churn (§13 item 3)
- A player-facing package for the mod and the sidecar, installable without a build toolchain
  and without asking a stranger to disable a security control
- A decided shipped default for a bare install, and an audit of every other default the
  package exposes to a machine the owner does not run
- A game-version and wire-version compatibility policy, and a way to move a fleet the owner
  cannot reach
- A support surface: an error taxonomy, a diagnostics command, and documentation a
  non-operator can follow alone
- The community playtest itself

Out of scope:

- **Returning a permanently rejected inbound organism** (§13 item 2). It needs one more
  message pair, and "held for an operator" stays an honest answer. Parked, and M5 does not
  unpark it — but note that the operator it is held for is now, sometimes, a stranger.
- **The control surface.** All three of its blockers are M5 items and none of them is the
  design itself. Decision 2 proposes deferring it; A43 states why deferring is cheap.
- **libp2p and direct peer-to-peer transport** (M6).
- **The `species-catalog` module, and corpse, pellet and egg payloads** (M7).
- **Census uploads to the archive** (D11).
- **Sleep and crash prevention.** D14's position is unchanged, and it is more defensible in
  public than it was on the LAN: a stranger's desktop sleeps whenever they close the lid.

*What M5 does not do*, near the end of this document, argues the boundary rather than
listing it.

## Scope the documents do not state

**Everything in this section is inferred. No document assigns any of it to M5, and none of it
is ratified.** It is here because the investigation that preceded this document went looking
for the M5 backlog and found that the authoritative lists — `contract-b-m4.md` §13 and
`contract-a.md` §12 — describe the *wire* work of going public and almost none of the work of
running something in public. Each item names where it is worked through.

| Inferred item | Why it is not optional | Worked through in |
|---|---|---|
| **Relay operations** — supervision, restart policy, monitoring, backup | The docs say "a hosted relay on a VPS" and stop there. A relay nobody restarts is not hosted | DQ2 |
| **A DNS name and certificate issuance/renewal** | TLS is in scope and needs a name; everything today hardcodes `192.168.1.227` and a port | DQ2 |
| **The forward receipt** | §13 item 6 declined it because "a relay restart is rare". On a VPS it is not | DQ2, Contract Changes 6 |
| **A signed or otherwise unblockable distribution** | `farend/README.md` currently asks for `Set-ExecutionPolicy Bypass` and `Unblock-File`. A stranger should refuse both | DQ4 |
| **A distribution channel** | No document names one. Thunderstore, Workshop and GitHub Releases are three different support models | DQ4, Decision 6 |
| **The shipped default for a bare install** | `MultiverseConfig.cs:374-379` makes an unconfigured mod export on all four edges. Nobody wrote the packaging note | DQ4, Decision 7 |
| **A public-safe defaults audit** | `--insecure-no-token`, the archive's `0.0.0.0` bind habit, the exclusion default and the save defaults were all chosen for a rig | DQ4 |
| **A game-version compatibility policy** | Steam auto-updates stay on, and "both computers run the same build, tell the owner" stops being an answer | DQ5, Decision 5 |
| **A fleet-update mechanism** | `contract-b/3.3` proved one class of minor breaks traffic until every peer upgrades, and peer application of a fix is unobservable | DQ5 |
| **Slot-space growth as a policy** | Slots never expire and are never reused (D8, D12). `maxSlotEverIssued` has never been tested at any scale | DQ6 |
| **Moderation and takedown for user-authored text** | Species names, world names and peer IDs are user text published to every subscriber. There is no report path and no operator action short of eviction | DQ7 |
| **A support surface for strangers** | One README, no error taxonomy, no diagnostics command. The owner is currently the diagnostics command | DQ8 |
| **Archive retention as a shipped rule** | The archive grows without bound *on purpose*, and peer count is the multiplier | DQ3, Decision 3 |

Two of these were created by this pass rather than found: the shipped default for a bare
install, and the public-safe defaults audit. Neither exists as a to-do in any document. The
first is a real deviation with a real cause — D17's default is correct for the rig and was
never assessed for a package — and `e2e/run-m4.sh:59-62` shows the rig noticing the hazard
and protecting only itself: *"THE VARIABLE STAYS EXPLICIT … so a future change to the mod's
default cannot silently move what this rig measures."* The rig wrote itself a guard and left
the packaging question for whoever packaged.

## Design Question 1 — TLS and per-peer credentials, and what they cost the wire

**They ship together, and the contract already says why.** §13 item 1: splitting them
"produces a half-secured relay that reads as secured". TLS without credentials encrypts a
wire on which any participant can impersonate any other. Credentials without TLS puts the
credential on the wire in the clear. Neither half is a milestone; the pair is.

**TLS is the cheap half.** It terminates at the relay's front door, changes no message, moves
no version, and needs one decision — whether the relay terminates it itself or sits behind a
reverse proxy that does. Everything expensive about it is operational: a name, a certificate,
a renewal that the relay survives (DQ2).

**Credentials are the expensive half, because they replace a rule rather than add a field.**
§3.1 today: one token for the whole map, sourced from `MULTIVERSE_TOKEN` or `--token-file`,
compared in constant time, and explicitly not an identity. §3.2's `4006` rule then lets a
second connection claiming an existing `peerId` evict the first. On a LAN of the owner's own
machines that composition is fine and the contract says so. On a public relay it is a
one-frame denial of service against any peer whose `peerId` you can read off the status page.

**The credential must bind to the `peerId`**, so that presenting somebody else's slot fails at
the handshake instead of succeeding at the eviction. That is the whole security property, and
it is worth stating as the acceptance test: a peer that presents a valid credential and
another peer's `peerId` is refused, and the legitimate peer never notices.

**Where credentials come from is the first thing in this project that needs an account of any
kind, and the recommendation is that it needs the smallest possible one.** Proposed: the relay
mints a peer secret at first claim and prints a join string containing the relay URL, the
`peerId` and the secret. No accounts, no email, no password reset. Recovery is slot handover
(§7.5), which already exists, already refuses to run while the old peer is live, and already
needs the authenticated admin path of §13 item 5 — so recovery costs nothing this milestone is
not already buying. The cost of the small design is honest and should be written down: a
stranger who loses their join string loses their world's identity until an operator hands the
slot over by name.

**The archive is a client of the same wire, so item 4 is one mechanism seen from the other
end.** §3.1's *Who* row already counts the archive among the token holders. A subscriber
authorisation rule is therefore not a second auth system — it is the same credential with a
different grant, plus an answer to a question the LAN never had to ask: what a subscriber is
allowed to see. That question got sharper twice after §13 was written. Since §16 a copied
`PEER_STATUS` carries every world's species census; since §19 it carries the mod version, the
save policy and the exclusion list. A public archive that copies every envelope to whoever
asks is publishing a fairly complete profile of a stranger's machine.

**The version calls.** On Contract A a bearer token is an additive field on an existing
handshake, so by §3.1's own test it is a **minor**: `contract-a/2.4`. On Contract B it is not
additive. §3.1's rule changes, the shared token stops being sufficient, and there is an
installed base — which is exactly the shape A41 and §3.1 call the expensive kind. **No
document decides whether that forces `contract-b/4`.** Decision 1 puts the call to the owner,
and D13's own argument is the reason it should be made now rather than later: pre-strangers
is the last moment a major costs a rebuild instead of a migration story.

## Design Question 2 — The hosted relay is a service, not a process

**Every operational fact recorded about this system describes a desktop.**
`dev_environment.md`, *The living deployment*, holds a seven-step bring-up ritual with an
ordered dependency on mounting a WSL volume, a read-only check of a Windows portproxy, a
per-instance log-starvation trap, a time-scale sweep, and a manual relaunch of a collector
that every reboot kills. None of that applies to a VPS, and none of its replacement exists.

**Name what has to exist, because "hosting" hides all of it:**

- **A supervisor.** The relay and the archive restart on exit and start on boot. This is the
  smallest item on the list and the one whose absence is most embarrassing.
- **Monitoring that speaks when nobody is looking.** The status page is excellent and it is a
  pull surface. A map that went to zero peers at 03:00 is currently discovered by someone
  opening a browser.
- **Backup of the two irreplaceable things.** The relay's `ring.json` is the slot-to-identity
  binding for every participant; losing it does not lose organisms but does lose everyone's
  address. The archive's three durable files are the record itself, and D11 makes them the
  seed of M7. Neither is reproducible from anywhere else.
- **A restart policy, written down.** Which restarts are routine, what a participant should
  expect during one, and what the operator does first afterwards.
- **A name, a certificate and its renewal.** ACME is the obvious answer and it has one
  non-obvious consequence: the relay must survive its own certificate rotating, which is
  another routine restart or a reload path.

**And the routine restart is what promotes the forward receipt.** §13 item 6 records the
declination and its reasoning exactly: a relay restart drops every outstanding `sent` record,
those entries fall back to the bounded 24-hour hold instead of re-routing in seconds, and M4
declined the fix "because a relay restart is rare and `--release-inflight` is one command".
**Both halves of that argument fail on a VPS.** Restarts stop being rare — deploys,
certificate rotation, kernel updates, the supervisor doing its job — and `--release-inflight`
is a command typed on the sender's machine, which after M5 is usually a stranger's. The
contract names the fix and its price: a receipt from the relay to the sender on each forward,
one frame per migration, turning the sender's own journal into the evidence. The condition
§13 item 6 set was "if this ever hurts". **The change of venue is what makes it hurt, and it
is knowable in advance rather than after the first bad night**, so the proposal is to build it
in M5 rather than wait for the incident that proves the sentence.

The counter-argument deserves a line, because it is the reason M4 said no: one extra frame per
migration on a relay whose whole design virtue is that it forwards frames and does nothing
else (D1). At the measured rates — around 300 to 500 crossings a minute across a six-slot map
— the receipt is a rounding error. At a public map's rates it is a real cost, and it should be
measured in WP3 rather than assumed away.

## Design Question 3 — Capacity, abuse limits, and what the archive keeps

**A limit only counts as a limit if D1 survives it.** The relay parses no body and indexes
nothing, and that property is load-bearing — it is why the archive is a separate service and
why the relay is replaceable by libp2p in M6. Every abuse limit M5 adds must therefore be
countable at the frame level: connections per source, frames per second per peer, bytes per
frame, claims per minute, genome requests per peer. A limit that requires reading a
`MIGRATION_PAYLOAD` body is not a limit the relay may have.

**One limit of the right shape already exists, and it is instructive.**
`contractb.GenomeRequestsPerMinute = 30` (`go/internal/contractb/contractb.go:230`), enforced
per peer in the archive's send path. It is a per-peer ceiling on a request a peer can issue as
fast as it likes, which is precisely the abuse-limit pattern — and it is a compiled constant,
reachable only by editing source. D20 already ruled on that shape of thing: *"a tunable an
operator cannot retune from the metric that measures it is not a tunable."* **Every public
limit must be a knob, and every knob a peer's behaviour depends on must be published**, as
D20 made the sidecar publish `inboundRatePerSimMinute` and `inboundRateBurst` on its stats
block. A peer that does not know the ceiling it is being measured against cannot be built to
respect it, and a support conversation about a limit nobody can read is unwinnable.

**Slot space grows monotonically and the rule that makes it grow must not be relaxed.** D8 and
D12 never reuse a slot number, and the reason is custody, not tidiness: `SLOT_VACANT` is a
*permanent* answer and therefore a valid proof of non-delivery, which is what lets an orphaned
journal entry re-route at all. Reissuing addresses would silently convert that proof into a
lie. What can be bounded instead is the *position*: D12 and D13 separate the address from the
coordinate exactly so that positions can move while addresses never do. An abandoned
reservation can be released — `--release-slot` already does it, and already prints the held
entries first — leaving a hole that both axes route around, while the address stays retired
forever. What M5 owes is not a new mechanism but a policy: **when an operator releases a
position that will never be filled again, and by what authenticated path** (§13 item 5).
`maxSlotEverIssued` has never been tested at any scale and should be, in WP5, before people
are involved.

**Every join re-publishes the map, and the live deployment already shows that this is not
free.** Slot 6 issued 64 placement claims in one day against two or three from each local
slot, each one a re-claim with `reason=updated` as its measured time scale moved — not a
reconnect, and not a defect, but 64 epochs. On a public map with heterogeneous hardware,
`PEER_STATUS` broadcast volume is a capacity question in its own right, and the coalescing
window §13 item 3 names is where it is answered.

**Retention is the item nobody owns, and it has about three months on the clock.**
`system_decomposition.md` §5 and `dev_environment.md`, *The disk budget*, agree: the ledger
and the genome store grow without bound on purpose, nothing may evict from them
(`contract-b-m4.md` §10, §20), and the measured 97 MB/h across the whole rig is roughly three
months on the 251 GB volume. *"Somebody has to decide what the archive is for before that runs
out."* A public archive scales with peer count, so the three months becomes weeks at a dozen
peers. The graduation question D6 leaves open — whether M7's `species-catalog` seeds from the
archive or supersedes it — stops being an M7 question the moment the disk is the binding
constraint. Decision 3 puts it to the owner. **What M5 must ship regardless of the answer** is
the honest arithmetic in the operator's hands: published per-peer growth rates and a sizing
procedure, so that whoever runs the relay knows what they signed up for before the volume
fills rather than after. The 2026-08-08 ENOSPC outage is the argument, and it happened on a
volume one person was watching.

## Design Question 4 — Packaging, and the default a bare install ships with

**The join kit is the starting point and it is honest about being a kit for one known
person.** `farend/README.md` asks its reader to obtain a token file from the main computer,
type an IP address, run `Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`, and
`Unblock-File` everything that came out of the zip. Each of those is a step a stranger should
refuse, and the last two are a security control being switched off by instruction. The
mechanics underneath are good — `setup-farend.ps1` finds Steam's install, verifies the game
build, installs BepInEx and the plugin, stores the token and writes start and stop scripts —
so M5's packaging work is mostly about how the artifact arrives and who vouches for it, not
about what it does once it runs.

**Distribution is a support-model choice, not a hosting choice**, which is why Decision 6 puts
it to the owner rather than recommending a URL. A mod site, a Workshop item and a GitHub
release differ in who updates a player's install, whether an update can be forced, and where a
confused person goes to complain. That last one matters more than it sounds, because DQ8 is
about the same population.

**Signing is the part that cannot be argued away.** A signed installer removes the two
commands above; an unsigned zip does not, whatever the README says. If signing is not
available, the honest alternative is a delivery path that does not carry the mark of the web,
not a README that talks a stranger through disabling execution policy.

**The shipped default for a bare install is a to-do that had to be created rather than
found.** `MultiverseConfig.ParseExportEdges` (`MultiverseConfig.cs:374-379`): an empty or
unset `MULTIVERSE_EXPORT_EDGES` yields all four edges with `Enabled = true`, commented in the
source as *"D17's default. Nothing configured means the whole perimeter, not silence."* The
literals `none` and `off` are the only way to keep the client off (`MultiverseConfig.cs:18-21`,
`30-33`, `153-156`), and `dev_environment.md` records the change as *"Unset now means all
four, not 'off'."* That default is correct for the rig it was designed for, where every
install is deliberate, and **no document assesses it for a package**. On a public package it
means installing the mod puts a player's world on the map with no further action.

**The resolution is less alarming than the framing, and it is worth working through rather
than reflexively flipping the default.** With per-peer credentials (DQ1), an unconfigured
install has no relay URL and no credential, so it cannot connect regardless of what its edge
set says. The four-edge default then stops being a safety question and becomes a semantics
question: what does a player who has configured a relay but not an edge set mean? D17's answer
— the whole perimeter — is the right answer for that player, and it is the answer the mod
exists to give. **The actual to-do is therefore to state the shipped default, once, in the
package and in the documentation, and to keep the rig's explicit-variable discipline** so that
a future change to the mod's default cannot silently move either what the rig measures or what
a stranger's world does.

**The public-safe defaults audit is the second created item, and it is four specific
defaults:**

| Default | Today | Why it needs a decision |
|---|---|---|
| `--insecure-no-token` | The relay refuses to start with no token unless this flag is passed, and then logs one loud warning per connection (§3.1) | The flag is documented as test-rig-only and nothing enforces that. A public build should make it impossible, or make it impossible to miss |
| The archive's HTTP bind | Compiled default `127.0.0.1:8796` (`go/internal/archive/main.go`, the `--http` flag); the rig passes `ARCHIVE_HTTP=0.0.0.0:8796` and the runbook says to | The compiled default is right and the *habit* is in the bring-up instructions. A hoster copies the runbook, not the source |
| `MULTIVERSE_MIGRATION_EXCLUDE` | Defaults to `Basic bibite` (`MigrationExclusion.cs:55`), read on presence, so an explicitly empty value disables the policy | D18 chose that default to keep founder stock off the lanes. A stranger who sets the variable empty floods a public map with seed genomes, and the census will show it as normal |
| `MULTIVERSE_SAVE_*` | `SaveMinutes` 10, `SaveKeep` 6, `SaveOnQuit` true (`MultiverseConfig.cs:80-81`, `104-106`) | Six retained saves of a world is a footprint on a disk the player did not budget, and the save interval is also the stall cadence — see Risk 3. **These three are now what the owner's own deployment runs** (2026-08-10): the rig's `2`/`4` override is gone, so the audit and the rig finally measure the same numbers |

## Design Question 5 — Version compatibility, and a fleet the owner cannot reach

**Two version axes, and the docs have a hard-won lesson on each.**

**The game.** `setup-farend.ps1` refuses to install against a mismatched build, and Steam
auto-updates stay on. Today that composes into a workable answer: both computers run the same
version, and if they do not, the owner is told. With strangers it composes into nothing — a
Steam update lands on some players and not others within hours, and the mod is a Harmony patch
against a specific assembly. `bb8-schema`'s cross-version conversion is listed as
unresearched in `system_decomposition.md`'s research table, so a mixed-version map is not
currently a thing anyone can reason about. Decision 5 asks for the policy: a tolerance window,
a forced-update path, or a stated refusal.

**The wire, where the lesson is sharper.** Compatibility is on the major alone, a minor is a
capability statement rather than a negotiation, and a feature is detected by the presence of
its field. That rule held for three consecutive minors — the far end sat one behind and kept
exchanging organisms — and then `contract-b/3.3` broke the streak.
`dev_environment.md`, *The minors*, records what happened: a pre-3.3 sidecar validates
`MIGRATION_PAYLOAD.exitEdge` against `E`/`N` and answers `W` with a **permanent**
`MALFORMED_MESSAGE` NACK, so an upgraded neighbour's west and south exports were refused,
bounced, and re-exported. The two lanes into the stale peer ran at ~40 hops/min against ~4–6
everywhere else, two other slots pinned at `inboundQueueMax`, and those slots started
answering their own senders with `OVERLOADED`. Nothing was lost; the map was simply not
operationally complete until every peer upgraded.

**The lesson as the document states it is the design input:** *"a stale value range in a field
both sides already exchange breaks traffic; a stale absent optional field only ever costs a
number on a page."* A public map cannot tell those two apart by itself, and the relay is the
only party that sees every peer's claimed version at once.

**So the proposal is a published minimum-version rule enforced at the relay handshake, plus a
way to move the fleet — and one caveat that must be written into the contract beside it.** A
version a peer *claims* is attacker-chosen text. §13 item 7 already forbids reading
`contractAVersion` as a capability decision for exactly this reason. A minimum-version gate is
therefore a **compatibility control and never a security control**: it keeps honest stale
peers off a map they would degrade, and it stops nobody who edits a string. Both halves have
to be said in the same paragraph or an implementer will assume the first implies the second.

**And note what makes this urgent rather than tidy.** The peer that suffers from staleness is
not the peer that is stale, and application of a fix on somebody else's machine is
unobservable from here (`dev_environment.md`, *The far end*). At six peers you ask the
operator. At sixty there is nobody to ask.

## Design Question 6 — Placement under churn, and a map with no operator

**M4 placed six known peers and spliced one newcomer, by hand, once each.** §13 item 3 says
so and names what churn will stress first: auto-placement, the coalescing window, and
`maxSlotEverIssued` growth. The relay has `--reserve-slot <peerId>[@<col>,<row>]` and honours
an advisory `insertAfterSlot`; what it does not have is a rule for a peer nobody expected.

**The placement rule follows from the shape the map is required to keep.** A ragged rectangle
is illegal (M4 Question 6), every position in the rectangle exists, and an empty position is a
hole with a defined bypass. A newcomer therefore either fills a hole or extends an axis, and
**holes first** is the only policy that keeps the rectangle dense. It also composes with DQ3's
slot-space answer: a departed stranger's *position* becomes fillable while their *address*
stays retired forever, which is precisely the split D12 and D13 built.

**Extending an axis is the expensive case and needs a stated rule**, because one newcomer
creates `height` positions and fills one of them. A map that extends on every join grows a row
of holes per join, and route-around walks them all. Proposed: extend only when no hole exists,
and extend the axis that keeps the rectangle closest to square, because the cycle length on
each axis is what mixing depends on.

**Continuous churn also stresses two things that are invisible on a stable map.** The
coalescing window — how many joins and departures collapse into one `PEER_STATUS` broadcast —
and the epoch rate, which DQ3 shows is already non-trivial at six peers because a peer whose
measured time scale wanders re-claims its position. Both are measurable before any stranger is
involved, and WP5 proposes measuring them with a synthetic churn harness rather than
discovering them during the playtest.

**One consequence should be stated plainly rather than discovered:** an organism routed around
a bypassed slot never sees that world, so a map with continuous churn is a map with a
continuously shifting cycle. That is not a defect — D12 chose it — but it means the genetic
mixing a public map produces is not the mixing a stable map of the same size produces, and
whoever reads the archive later should know which one they are looking at.

## Design Question 7 — Attacker-chosen text, and who owns it

**The inventory is already written.** §13 item 7: species names are attacker-chosen text of up
to 64 bytes per entry and 32 entries per peer, landing in a broadcast and then in a renderer;
`migrationExclude` is another attacker-choosable string array on the same path; `modVersion`
and `contractAVersion` are two more free-text fields. Add the two the contract does not count
because they predate the concern: `peerId`, and the world name a player chose.

**The split of responsibility is stated and must now be implemented on both sides.** The
wire's answer is the shape check and the cap, and it is done. The renderer's answer is its own,
and §13 item 7 is blunt about it: *"a page that interpolates a name into HTML without escaping
it has a defect this contract cannot fix for it."* **An escaping rule written in a contract is
not code**, so M5 owes a test — a peer that reports a species named with markup, asserted
against the rendered page and the terminal output of `ringstat`, in CI rather than by eye.

**Beyond injection there is moderation, and this is genuinely new ground for the project.** A
species name is a string a player typed or the game generated, published to every subscriber
and rendered on a page anyone with the URL can read. There is no report path, no takedown
path, and no operator action between "ignore it" and "evict the peer". The minimum that costs
nothing on the wire is an **operator-side deny list applied at render**: the page and
`ringstat` suppress a string without evicting the world that produced it, needing no peer
cooperation and no contract change. Eviction stays available through the authenticated admin
path (§13 item 5) for a peer that will not stop.

**And there is a conflict here that M5 must not paper over.** D11 and §10 make the archive's
ledger a record from which nothing may ever evict — *keep the hash forever*. A takedown
against the *view* is achievable; a takedown against the *record* contradicts the rule the
archive is built on. **M5 should therefore promise removal from the view and say explicitly
that it does not promise removal from the record**, rather than implying a deletion the design
forbids. If the owner wants the stronger promise, that is a change to D11's never-evict rule
and belongs in a decision row, not in a support reply.

## Design Question 8 — What a stranger can diagnose alone

**Today's diagnosis surface is the status page, `ringstat`, five kinds of log, and the
owner.** A stranger has their own sidecar log and nothing else. The failure modes they will
hit are all on the record already, and they sort into an unhelpful pattern:

| Symptom | Cause | Who can see it |
|---|---|---|
| HTTP 401, backoff pinned after five tries | Wrong or missing credential (§3.1) | The peer, clearly |
| The setup script refuses to install | Game build mismatch (`setup-farend.ps1`) | The peer, clearly |
| Game runs, `modConnected` false, `configuration failed` in the BepInEx log | The config-race trap; the fix is a restart, one instance at a time | The peer, with the runbook |
| Game runs, `modConnected` false, **no** `configuration failed` anywhere | Log-file starvation; a different remedy entirely | The peer, only if they know both traps exist |
| Every rate reading is wrong by a factor of seven | The world is at the wrong time scale | Nobody, unless they sweep it |
| A neighbour's lanes run at ~40 hops/min and their queues pin at `inboundQueueMax` | **This peer** is on a stale build | The operator, and nobody else |

**The last row is the argument for the whole work package.** The peer that suffers is not the
peer that is stale, so a stranger cannot debug a problem whose cause is in someone else's
install, and the operator is the only party positioned to see both ends. A support surface
that assumes each participant can diagnose their own machine is wrong about this system's most
likely public failure.

Proposed, and each piece is small:

- **An error taxonomy** in which every refusal names the remedy *and who must act*. The second
  half is the novel part and the table above is why.
- **`multiverse-sidecar --diagnose`**, which runs the checks the runbook currently performs by
  hand: relay reachable, credential accepted, mod connected, game version, wire version, edge
  set, time scale, journal state, and disk headroom for the genome cache.
- **The page telling a peer what the map thinks of it.** A participant should be able to read
  their own slot's liveness, lane states, paced depth and last save without an operator
  reading it to them — which is mostly a presentation change, since every field already
  exists.

## Inherited threads

M5 does not start from a clean rig. Seven threads are open at the boundary, and each one bears
on the milestone rather than merely coexisting with it.

- **The save-stall budget is being missed today, on M4's own signed-off bar.** The owner set
  2 000 ms (D14, M4 Risk 3), the exit test measured 241–538 ms, and the deliverable is recorded
  as passed. The living deployment's logs disagree, and the breach is written up separately as
  a watch item in `dev_environment.md`, *The living deployment → Watch items*. **This document does not restate that
  analysis and should not be read as a second opinion on it.** What M5 owes is the
  consequence: M4 Risk 3 named the escalation — *"a different cadence, or a save path that does
  not block the tick"* — and M5 is the milestone in which the cadence stops being the
  operator's to choose, because it ships as a package default on machines nobody here can
  retune. See Risk 3.

  **The owner took the first half of that escalation on 2026-08-10, and it changes what M5
  inherits — not the risk, but the baseline.** The five local worlds now run the mod's own
  shipped `saveMinutes=10 saveKeep=6` (the rig had been overriding them with an exit-test 2 and
  4), and `contract-a.md` §20 A45 raised `heartbeatTimeoutMs` from 3 500 to 13 000 so a blocking
  save stops costing a Contract A session. **Two consequences for this milestone.** First, the
  cadence M5 must audit is now the *package default*, not a rig setting — DQ4 is auditing the
  number a stranger will actually get, which is what Risk 3 asked for. Second, the deadline is
  no longer a hidden constraint on it: a package that saves a small world in 300 ms was never
  near 3 500 ms either, but M5's own defaults audit can now reason about the save budget without
  also reasoning about the disconnect. **The 2 000 ms bar itself was not moved**, and the
  remaining escalation — *a save path that does not block the tick* — is still unspent.
- **Time scale is report-only on the wire.** `contract-b-m4.md` §18 carries `timeScale` as a
  measured reading copied from `HEARTBEAT` and *never computed*, and there is no message that
  sets it. On the LAN the remedy is one dev command — `run-m4-lan.sh send 2 timescale 5`, which
  corrected a settled world that had drifted to ×34.7 hours after a clean bring-up. On a public
  map there is no such command and no way to obtain one without the control surface. See
  Risk 9.
- **`genomeGaps` changes character when peers leave forever.** The backlog is throughput-limited
  rather than leaky — the collector's series shows long stretches at zero, a rise that tracks
  the crossing rate and a monotone drain when traffic falls — and `dev_environment.md`'s watch
  item now holds that reading and its resolution. The M5 change is not the rate but the
  ending: today "the source sidecar forgot the genome" has exactly one instance in the entire
  log history, because every peer comes back. After M5 a peer that leaves for good makes that
  the normal case, against a fetch ladder that runs from one minute to daily and a sidecar
  cache capped at 30 days and 2 GiB. See Risk 7.
- **The archive's own durability is now level with the sidecar journal's, as of 2026-08-09.**
  §20's all-or-nothing append rule had been written for `internal/journal` only, and the
  archive's ledger — a separate implementation — carried the same defect and the 2026-08-08
  splice that proves it. Both halves are closed: the append truncates back, and replay skips a
  line it cannot parse rather than stopping at it, which is the form the rule has to take for a
  file nothing ever compacts. The point for this milestone survives the fix and is structural:
  **M5 is where the archive stops being the operator's private recorder and becomes a service
  other people depend on**, so every rule of this kind has to be checked in both
  implementations rather than in the one it was written for.
- **`m4_findings.md` is unwritten.** It is the one open M4 deliverable. Its inputs —
  `m4_considerations.md`, `e2e/logs-m4-lan/` and `m4_portal_findings.md` — are also the best
  available baseline against which a public map's behaviour will be compared, so writing it
  before the playtest is worth more than writing it after.
- **`phase5far` has never been run.** Cross-LAN pacing is confirmed locally and never across
  the link, because the run needs two commands typed at the second computer and D9 forbids the
  rig to send them. M5 makes that permanent: after M5 *every* peer is a peer the rig cannot
  drive, so a test that needs a person at the far end needs a person who volunteered.
- **The capture band still has no velocity magnitude floor, and A38 reopened the item at its
  original priority.** `contract-a.md` §12 item 7's cheapness argument rested on the one-way
  lane, and D17 removed the lane. See Risk 8 and Decision 9.

## Risks

### Risk 1 — A half-secured relay reads as secured

The failure is not technical, it is a claim. A relay with TLS and a shared token looks secured
in every screenshot and is not: any participant can still present any `peerId` and evict its
holder. §13 item 1 anticipated exactly this and made the pairing a rule.

Mitigation: ship them together, and make the acceptance test adversarial rather than
functional. A peer presenting a valid credential with another peer's `peerId` must be refused
at the handshake, and the legitimate peer must not observe anything at all. A test that only
proves the happy path proves the wrong thing here.

### Risk 2 — The exit test depends on people, and people are not a rig

Every milestone so far closed with a script. M5's exit test is *"a handful of strangers' sims
exchanging organisms for days without operator intervention"*, and every word of that is
outside the harness. Participants drop out, close laptops, upgrade Steam mid-run and lose
interest on day two. A run that ends early is the likely case, not the exceptional one.

Mitigation: recruit more participants than the bar requires; define the pass on the map's
behaviour rather than on attendance, so that a departure is a churn test instead of an
aborted run; and instrument enough that a run which ends on day two still yields the
measurements. Decision 8 asks the owner to set the bar, because "a handful" and "days" are
not testable as written.

Accepted cost: the exit test cannot be re-run cheaply. Unlike every M4 phase, it costs other
people's goodwill each time, which argues for doing the abuse cases and the churn harness
before the playtest rather than during it.

### Risk 3 — The save stall is a local annoyance here and a first impression there

On the owner's rig a save that overruns its budget costs simulation throughput and is
observed by someone who understands it. On a stranger's machine the same stall is a visible
freeze, at a cadence the package chose for them, in the first hour they run the mod. It is
not a correctness risk — nothing on the wire depends on it — which is exactly why it can be
under-weighted.

Mitigation, in order of cost: make the cadence and retention obviously tunable and documented
in the package (DQ4's defaults audit); measure the stall at the population and world size a
new player actually runs, which is far smaller than this rig's; and take M4 Risk 3's named
escalation only if those two are not enough. Note that `WorldSaver.cs:68-73` already records
why a coroutine save was rejected, so "just make it async" is not an available answer.

**The rig now runs the same cadence a stranger will get** (2026-08-10): the owner dropped the
rig's `saveMinutes=2 saveKeep=4` override, so the five local worlds are on the shipped `10`/`6`.
That is a **better** position for this risk than the one it was written in — every stall this rig
measures from now on is a stall at the package's own interval, on a world far bigger than a new
player's, which is the conservative direction. It does **not** retire the risk: the freeze a
stranger sees is one ten-minute-spaced 300 ms hitch rather than this rig's multi-second one, and
whether that reads as a bug is still a first-impression question rather than a measurement.

**One thing that did move is off this risk's plate.** A blocking save used to cost a Contract A
`4004` as well as a freeze, because the heartbeat rides the thread the save blocks; `contract-a.md`
§20 A45 raised `heartbeatTimeoutMs` to 13 000 and that half is now closed for every mod on the
wire, packaged or not. What M5 inherits is the freeze, not the disconnect.

The live breach is a watch item elsewhere and its resolution belongs there. What M5 must not
do is ship the current defaults to strangers on the strength of a measurement taken on a
different machine at a different scale.

### Risk 4 — Route-around hides a dead peer, and now nobody is watching

M4's Risk 5 in its public form. A healed map keeps working, which is the point and also the
failure mode: a participant's world can be gone for a day while the map looks healthy. The
status page already shows every bypass with the time it went dark, and `ringstat` prints the
same list. **What does not exist is anyone reading them.**

Mitigation: this is DQ2's monitoring item, and it is the reason that item is not optional. A
bypass that persists past a threshold should reach the operator without the operator asking.
The secondary mitigation is social — a participant whose world dropped out should be able to
tell that it dropped out, which is DQ8's page work.

### Risk 5 — The archive fills, on somebody else's timetable

Three months of headroom is a measurement of *this* deployment at six worlds. Peer count is
the multiplier on the genome store and the crossing rate is the multiplier on the ledger, and
a public map moves both at once. The 2026-08-08 ENOSPC outage is what running out looks like:
it stopped every genome write in the rig and left durability damage that took days to
understand.

Mitigation: publish the per-peer growth arithmetic so the hoster sizes the volume from a
number rather than a hope; monitor free space as a first-class signal, not as part of a
weekly look; and settle Decision 3 before the volume rather than after. **Accepted cost if
Decision 3 says "keep everything":** the operator is signing up for a disk bill that grows
with the community's success, and that should be an informed signature.

### Risk 6 — A stale peer breaks its neighbours' traffic, not its own

Measured, not hypothesised: the `contract-b/3.3` episode in `dev_environment.md`, with its ~40
hops/min bounce loop and two innocent slots pinned at `inboundQueueMax`. On the LAN it was
resolved by re-taking a bundle. On a public map the stale peer has no incentive to act,
because their world is fine.

Mitigation: a relay-side minimum-version gate (DQ5), the fleet-update mechanism behind it, and
an operator view that names the peer *causing* the degradation rather than the peers
displaying it. Accepted cost: the gate trusts a self-reported string and therefore only
handles honest staleness. That is the majority of cases and it must be documented as the
limit of the control, not sold as more.

### Risk 7 — A departed stranger's genomes become permanently unfetchable

An entry leaves the archive's `pending` set only when the genome arrives. A peer that leaves
forever converts a throughput-limited backlog into a permanent one, and the retry ladder then
walks the map indefinitely against a blob nobody holds. The sidecar cache is capped at 30 days
and 2 GiB, so even a returning peer may no longer have it.

Mitigation options, in ascending cost: bound the ladder and record the abandonment as a fact
the operator can count, instead of retrying forever; have the archive fetch eagerly rather than
lazily for peers it has not seen recently; or let any peer that holds a genome serve it, since
the store is content-addressed and the requester can verify what it gets. The third is the
real fix and it edges toward M6's territory, so M5 should probably buy the first and measure
whether the second is needed.

Accepted cost either way: the archive's lineage graph will have holes for departed peers, and
D11's *known limit* framing already prepares for a record that is incomplete by construction.
It should be stated rather than discovered by whoever reads the archive in M7.

### Risk 8 — The wire goes public carrying a reopened design item

`contract-a.md` §12 item 7: the capture band's direction test has no magnitude floor, so an
organism loitering in a band can export on a single tick of jitter. M3 accepted it because the
one-way lane bounded the consequence to "the organism moved on". **A38 withdrew that
argument** when D17 made every lane two-way: an organism that crosses east, turns around and
crosses back west is now a legal round trip, and one loitering on a shared border can
oscillate across it. The item is reopened at its original priority and unscoped.

On this rig that is noise. On a public map it is a pair of strangers' worlds producing
crossings forever, spending each other's pacing budget and writing a permanent record of it
into a ledger nothing may evict from. The fix A38 sketches is a floor on the outward component
**over several ticks**, not on one sample, and it changes a rule the mod enforces on every
organism on every tick — which is why it is a decision (Decision 9) rather than a work item.

Mitigation if the answer is "not in M5": measure it during the playtest, because a populated
public map is the only place the rate question can be answered, and D13's argument means the
next comfortable moment to change a mod-side rule is worse than this one.

### Risk 9 — Time scale is unfixable on a stranger's world, and it corrupts the readings

A world at ×34.7 inflates every per-simulated-minute series it produces. Those series are not
decoration: D20 sized the pacing limit from them, twice, and revised it twice more when the
measurements moved. On a public map the operator can read `timeScale` per peer and can do
nothing about it, because there is no message that sets it and the field is explicitly a
report (`contract-b-m4.md` §18, B16).

Mitigation: read rates per peer with the time scale beside them and refuse to aggregate
per-simulated-minute series across peers running at different speeds — an aggregate over
mixed time scales is not a slow number, it is a meaningless one. Accepted cost: some
participants will run fast worlds and their pacing behaviour will differ from everyone else's.

This is also the strongest live argument for the control surface, and Decision 2 should be
read with it in view: the case for deferring the control surface is that it is cheap to add
later; the case against is that M5 is the first milestone where its absence is felt by
somebody other than the owner.

### Risk 10 — The operator is one person, and hosting is a standing commitment

Every previous milestone ended. This one starts something that does not: a service other
people depend on, with a support load attached. The project's own record shows what unattended
means in practice — a collector that no reboot restarts, a bring-up that is a seven-step hand
procedure, a full-disk outage discovered by its symptoms.

Mitigation: DQ2's supervision and monitoring are the technical half. The other half is
Decision 4, which is not a technical question at all. **A playtest that succeeds creates an
obligation**, and the honest options include running the relay for a bounded, announced
period, or publishing the relay so somebody else can run one. Deciding that after the
strangers arrive is worse than deciding it now.

## Contract Changes Needed

This pass does not rewrite the contracts. The list below is the design input for the
implementation wave, and the **Status** column separates what this document proposes to commit
from what it proposes to leave alone.

| # | Document | Change | Source | Status |
|---|---|---|---|---|
| 1 | `contract-b-m4.md` §3.1 | Per-peer credentials replace the one shared token. The credential binds to the `peerId`, and §3.2's `4006` eviction requires it. State what a peer does when its credential is rejected. | §13 item 1 | **M5**. Version call open — Decision 1 |
| 2 | `contract-b-m4.md` §3 | TLS: the scheme, certificate expectations, what a client does with a certificate it cannot verify, and what a certificate rotation looks like to a connected peer. | §13 item 1 | **M5** |
| 3 | `contract-a.md` §5, §12 item 1 | A bearer token on Contract A. Additive on an existing handshake, so a **minor**: `contract-a/2.4`. | §12 item 1 | **M5** |
| 4 | `contract-b-m4.md` §10, §13 item 4 | A subscriber authorisation rule for the archive, and what a subscriber is permitted to see of another peer's stats block now that it carries the census, the mod version, the save policy and the exclusion list. | §13 item 4 | **M5** |
| 5 | `contract-b-m4.md` §7.5, §13 item 5 | An authenticated admin path for slot release, handover and eviction. Keep the printed held-entry report; the acts stay deliberate. | §13 item 5 | **M5** |
| 6 | `contract-b-m4.md` §5.2, §13 item 6 | The relay forward receipt: one acknowledgement per forward, making the sender's own journal the evidence. | §13 item 6 | **M5**, promoted — DQ2 |
| 7 | `contract-b-m4.md` §13 item 7 | Restate the renderer's escaping obligation as a testable rule, and repeat the `contractAVersion` prohibition where an implementer of a page will read it. | §13 item 7 | **M5** |
| 8 | `contract-b-m4.md` §7.2 | Auto-placement under churn: holes before axis extension, which axis extends, the coalescing window, and a bound on `PEER_STATUS` broadcast rate. | §13 item 3 | **M5** |
| 9 | `contract-b-m4.md` §3.x (new) | Capacity limits as a published table — connections, frames, claims, payload bytes and genome requests per peer — every one a knob, and every one a peer depends on published on the stats block. | Inferred; D20's knob rule | **M5** |
| 10 | Both documents | A minimum-version rule at the relay handshake, stated as a **compatibility** gate and explicitly not a security control. | Inferred; `dev_environment.md`, *The minors* | **M5** |
| 11 | `contract-a.md` §12 item 8 | `"blob_dropped_for_size"` is defined and never emitted. One additive OPTIONAL field on `parents[]` closes it. | §12 item 8 | **M5** — cheap now, a migration story later |
| 12 | `contract-a.md` §12 item 9 | A startup refusal in the sidecar for a non-grid `exportEdges` set. The sidecar knows the map; the mod must not. | §12 item 9 | **M5** — it bites hardest with strangers |
| 13 | `contract-a.md` §12 item 6 | Contract debt A5: assess and either close it or restate why it stays moot. Its second reason — "the mod records the edge" — is an obligation somebody could drop. | §12 item 6 | **M5** — assess only |
| 14 | `contract-a.md` §4.3.1, §12 item 7 | A velocity magnitude floor on the capture test, over several ticks rather than one sample. | A38 | **Owner call** — Decision 9 |
| 15 | Both documents | The version calls: `contract-a/2.4` for item 3, and major-or-minor on Contract B for item 1. | §3.1, A41, D13 | **Owner call** — Decision 1 |
| 16 | `m1_considerations.md`, `m2_considerations.md`, `m2_findings.md`, `m3_considerations.md`, `contract-b-m2.md` | Older documents still say "M5" where D16's renumbering makes them mean M6 or M7. Correct them in this wave, as D16's own item 13 corrected the previous round. | D16 | **M5** — documentation |

Items 11, 12 and 13 are not urgent and are listed because this is the last milestone in which
they are cheap. Every one of them is an additive change or a local refusal today, and the same
change after strangers run the build costs a migration story for people the owner cannot reach
(`contract-a.md` §15 A23).

## Work Packages

Eight packages. WP1 gates the wire work, WP2 gates WP3 and WP4, and WP8 cannot start until
everything else has run against a synthetic map first.

Nothing is started. The status lines below are placeholders for the record each package will
carry, in the form M4's packages carry theirs.

### WP1 — The contract amendments

**Depends on:** this document, and Decisions 1, 2 and 9.
**Needs the game:** no.

- The sixteen items of *Contract Changes Needed*
- One worked example for each changed or added message
- The version decision, applied, with a migration note for the living deployment — which is
  itself a peer that has to move
- The stale-milestone correction of item 16

Write WP1 before any code. M2 and M3 both paid for the alternative, and M4 said so.

### WP2 — Transport security

**Depends on:** WP1.
**Needs the game:** no.

- TLS on Contract B, at the relay or in front of it
- Per-peer credentials, bound to `peerId`, with the join-string issuance of DQ1
- The archive as an authorised subscriber, and the visibility rule of Contract Change 4
- A bearer token on Contract A
- The adversarial acceptance tests of Risk 1, not only the functional ones

### WP3 — The hosted deployment

**Depends on:** WP2.
**Needs the game:** no.

- A VPS, a DNS name, and ACME issuance and renewal the relay survives
- Supervision and boot-time start for the relay and the archive
- Monitoring that reaches a person: peers lost, bypasses that persist, free disk, error lines
- Backup for `ring.json` and the archive's three durable files
- A written restart policy, and the participant-facing statement of what a restart looks like
- **The forward receipt**, and the measurement of what it costs per migration at rate

### WP4 — Capacity, abuse limits and the admin path

**Depends on:** WP1, WP2.
**Needs the game:** no.

- The published limit table of Contract Change 9, every entry a knob, published on the stats
  block
- Frame-level enforcement only — nothing that requires the relay to read a body (D1)
- The authenticated admin path: release, handover, eviction
- The operator-side render deny list of DQ7
- The three cheap closures: Contract Changes 11, 12 and 13

### WP5 — Placement and route-around under churn

**Depends on:** WP1, WP2.
**Needs the game:** no.

- Auto-placement: holes first, then the axis rule
- The coalescing window, and the `PEER_STATUS` broadcast bound
- `maxSlotEverIssued` growth, tested rather than assumed
- The slot-space policy of DQ3: what an operator does with a position that will never be
  filled again
- **A synthetic churn harness** — peers joining, leaving, returning and never returning, at
  rates no human rig can produce — run to exhaustion before WP8 involves anybody

### WP6 — The package

**Depends on:** WP2. Input: `farend/`.
**Needs the game:** yes.

- A player-facing installer for the mod and the sidecar, with no build toolchain and no
  instruction to disable a security control
- Signing, or a delivery path that does not need it (Decision 6)
- **The shipped default for a bare install**, decided and stated in one place (Decision 7)
- **The public-safe defaults audit**: `--insecure-no-token`, the archive bind habit, the
  exclusion default and the save defaults
- The game-version policy of Decision 5, enforced by the installer as `setup-farend.ps1`
  already enforces its own
- An uninstall that leaves the player's game as it found it

### WP7 — The support surface

**Depends on:** WP1.
**Needs the game:** yes, for the mod-side failure modes.

- The error taxonomy of DQ8, in which every refusal names the remedy and who must act
- `multiverse-sidecar --diagnose`
- The page and `ringstat` telling a peer what the map thinks of it
- Renderer escaping, with the markup-species-name test in CI
- Documentation beyond one README: install, join, diagnose, leave

### WP8 — The playtest

**Depends on:** WP2, WP3, WP4, WP5, WP6, WP7.
**Needs the game:** yes, on other people's machines.

- Recruiting, and the bar of Decision 8
- The run, and the observation plan that survives an early ending
- The abuse cases of Exit Test Part 5, run by the operator against the live relay
- The record: per-peer archive growth, crossing rates, churn behaviour, and every support
  interaction that needed the owner

## Exit Test

`system_decomposition.md` states it in one sentence: **a small community playtest — a handful
of strangers' sims exchanging organisms for days without operator intervention.** The parts
below make that testable. "A handful" and "days" are Decision 8; the placeholders here are
this document's proposal, not a bar anybody has agreed to.

Proposed bar: **at least four peers that are not the owner's, at least 72 continuous hours,
and zero operator actions on any participant's machine.** An operator action on the *relay* is
allowed and counted — a restart under load is Part 4 — but anything typed on a participant's
computer at the operator's instruction is an intervention and fails the sentence.

### Part 1 — Strangers join without the owner

1. Each participant installs from the published package, using the published documentation
   only.
2. Each obtains a credential and joins the map.

The test passes when all of these conditions hold:

- Every participant reaches a live slot with no instruction outside the published documents.
- No participant is asked to disable a security control.
- Every step that needed the owner is recorded, with what it was.

### Part 2 — The map holds for days

3. Run for the agreed duration with no operator action on any participant's machine.

The test passes when all of these conditions hold:

- Organisms cross in both directions on every open lane, throughout.
- Exactly-once holds map-wide, counted by entity ID at the end.
- No placement, release or handover is performed by hand.
- Every automatic bounce is a fact on the page, and each is explained by the rule that caused
  it.

### Part 3 — Churn, including a departure that is permanent

4. Participants leave and return on their own schedules.
5. One participant leaves and does not come back.

The test passes when all of these conditions hold:

- A departure is routed around within one status broadcast, on both axes.
- A return reclaims the same slot number and the same position, with no operator action.
- No slot number is ever reissued.
- Held entries for the permanent departure bounce on the documented clock, and each organism
  exists exactly once afterwards.
- `genomeGaps` attributable to the permanent departure is bounded and counted, not retried
  forever in silence (Risk 7).

### Part 4 — The relay restarts under load

6. Restart the relay while traffic is flowing.

The test passes when all of these conditions hold:

- Every peer reconnects without operator help.
- Outstanding forwards are resolved from receipts rather than by waiting out a 24-hour hold.
- No organism duplicates, and none is lost beyond what D2 already accepts.

### Part 5 — The abuse cases, run by the operator

7. Against the live relay: a peer presenting another peer's `peerId`; a peer over each
   published ceiling; an oversized payload; a species name made of markup; a peer claiming
   `contractAVersion: "99"`; a flood of connections from one source.

The test passes when all of these conditions hold:

- The impersonation is refused at the handshake, and the legitimate peer observes nothing.
- Each ceiling refuses in the documented way, and the refusal names the limit.
- The markup name renders as text everywhere it renders — page and terminal.
- The claimed version changes no capability decision anywhere.
- The connection flood degrades the flooder and nobody else.

### Part 6 — The support surface earned its place

8. Every failure any participant hit is checked against the taxonomy.

The test passes when all of these conditions hold:

- Every failure appears in the taxonomy, with a remedy that worked.
- `--diagnose` identified the cause in each case where it ran.
- The count of interactions that needed the owner is recorded. It is a measurement, not a
  pass condition — but a large number means WP7 is unfinished, whatever the map did.

### Part 7 — The record

9. Capture per-peer archive growth, crossing rates, epoch rate, broadcast volume, and the
   disk arithmetic for the next milestone.

An unknown value reads as unknown. A zero reads as a measurement.

### Part 8 — The error sweep

The test passes when no log holds an unexplained error at the end of the run: relay, archive,
and every log a participant is willing to share.

## Deliverables

Nothing is delivered. This is the list the milestone will report against.

- The wire specification, from WP1, at whatever versions Decision 1 sets
- A relay with TLS, per-peer credentials and an authenticated admin path
- An archive with a subscriber authorisation rule and a stated visibility boundary
- A published capacity-limit table, every entry a knob and every relevant one on the stats
  block
- A relay forward receipt, with its measured per-migration cost
- Auto-placement and route-around proven against a synthetic churn harness before any person
  is involved
- A hosted deployment: VPS, DNS, certificate renewal, supervision, monitoring, backup and a
  written restart policy
- A player-facing package with a decided shipped default and a completed defaults audit
- A game-version and wire-version compatibility policy, enforced where it can be and
  documented where it cannot
- An error taxonomy, `multiverse-sidecar --diagnose`, and documentation a non-operator can
  follow alone
- Renderer escaping, with its test
- The community playtest, with its recorded result
- An `m5_findings.md`. M3 and M4 both left this deliverable open; M5 has more reason than
  either to close it, because its results come from people who will not be available to ask
  again.

## Decisions for the Owner

**None of these is decided. Every item below is a PROPOSAL awaiting ratification**, in the
form the previous milestones used: the recommendation is stated, the argument against it is
stated, and the owner's call is what settles it. Three of M4's recommendations were overridden
and the milestone was better for it.

**1. Does per-peer credentialing take Contract B to a major?**
Replacing §3.1's shared-token rule is not additive. §3.1's own framing and A41's test both
put "a changed rule with an installed base" on the expensive side. Against a major: the URL
path moves, the living deployment upgrades in lockstep, and a minor with a presence-detected
credential field might be made to work. For a major: **D13's argument, applied one milestone
later.** The breaking `exportEdge`→`exportEdges` change landed in M4 precisely because "a
breaking wire change after strangers run the build costs a migration story rather than a
rebuild", and M5 is the last moment that sentence is true. *Recommended: take the major, and
take it in WP1 so that everything else in the milestone is built on the wire that ships.*

**2. Does the control surface land in M5?**
All three of its blockers are M5 items — per-peer authentication, subscriber authorisation
and an authenticated admin path — so M5 is the first milestone in which it becomes possible.
A43 says deferring is cheap: a new message type is additive and costs a minor, and nothing in
§19 has to be undone. Against deferring: Risk 9 is the concrete case — the operator can watch
a stranger's world run at seven times everyone else's speed and can do nothing. For deferring:
the surface needs ordering and idempotency for a write that races a world load, semantics for
a disconnected target, authorization across machines D9 keeps undriven, and an audit trail,
and *"reversing `CONFIG_UPDATE` or making a stats field writable is not the cheap version of
that work; it is the same work with the questions skipped."* *Recommended: defer to M6, and
record Risk 9 as the accepted cost.*

**3. What is the archive for — and does M7's catalog seed from it or supersede it?**
About three months of headroom at six worlds, and peer count is the multiplier. Nothing may
evict from the ledger or the genome store (D11, §10, §20), no tunable caps either, and D6
leaves the graduation question open in the research table. This is an operator decision no
milestone owns, and a public archive forces it earlier than M7 would have. The options are
distinguishable: keep everything and size the disk; keep the ledger and prune genomes to a
horizon; or graduate now and let the archive become the catalog's ancestor in fact rather than
in principle. *Recommended: decide the retention rule in M5 even if the answer is "keep
everything", and ship the arithmetic to the hoster either way.*

**4. Who operates the public relay, and under what commitment?**
`system_decomposition.md`'s research table still lists this as open, and D9 answered it only
for the LAN. A playtest that succeeds creates a standing obligation with a support load
attached, on one person. The options include the owner running it for a bounded and announced
period, the owner running it indefinitely, or publishing the relay so other people run their
own maps. *Recommended: a bounded, announced commitment for the playtest, with the
publish-the-relay path prepared, so the answer to "what happens after" is not improvised in
front of participants.*

**5. What is the game-version compatibility policy?**
Steam auto-updates stay on and land unevenly. `bb8-schema` cross-version conversion is
unresearched. The choices are a tolerance window, a forced-update path, or a stated refusal to
run mixed versions at all. *Recommended: refuse mixed versions at the relay handshake for M5
and say so plainly, because a tolerance window rests on research that has not been done and a
forced-update path needs a distribution channel that has not been chosen.*

**6. What is the distribution channel, and is the sidecar signed?**
Thunderstore, Steam Workshop and GitHub Releases differ in who updates a player's install,
whether an update can be pushed, and where a confused person goes for help. Signing decides
whether the README still has to ask a stranger to bypass execution policy. *Recommended:
pick the channel before WP6 starts, because the fleet-update mechanism of DQ5 is built on
whatever it can do.*

**7. What default does a bare install ship with?**
`MultiverseConfig.cs:374-379` gives an unconfigured mod all four export edges and
`Enabled = true` — D17's default, chosen for a rig where every install is deliberate, and
never assessed for a package. DQ4 argues the safety half of this dissolves once credentials
exist, since an install with no relay and no credential cannot connect regardless. What
remains is a semantics call the package must state out loud. *Recommended: keep D17's default,
state it in the package documentation and in the installer's own output, and preserve the
rig's explicit-variable discipline so that neither the rig's measurements nor a stranger's
world can be moved by a future change to a default.*

**8. What is the exit-test bar?**
"A handful of strangers' sims exchanging organisms for days" is not testable as written, and
Risk 2 says a run will probably end early. *Recommended: at least four non-owner peers, at
least 72 continuous hours, zero operator actions on any participant's machine, with relay-side
actions allowed and counted. A shorter run reports what it proved rather than failing
silently.*

**9. Does the velocity magnitude floor land in M5?**
A38 reopened `contract-a.md` §12 item 7 at its original priority when D17 removed the one-way
lane that made it cheap. The fix is a floor on the outward component over several ticks, and
it changes a rule the mod runs on every organism on every tick. For M5: the wire and the mod
both go public, and after that a mod-side rule change reaches people the owner cannot contact.
Against: it is a **rate** question, and only a populated public map can answer it — which
means shipping the floor in M5 means guessing the threshold. *Recommended: do not ship the
floor; instrument it during the playtest so the threshold comes from a measurement, and accept
Risk 8's cost for the duration.*

## What M5 does not do

**It does not make the system safe against a determined adversary.** It makes it safe against
the ordinary hazards of being on the public internet — impersonation between participants,
accidental floods, a stranger's page being used to inject markup into another stranger's
browser. A participant who wants to lie about their population, run a modified sidecar, or
spend the map's capacity on nothing is bounded by the published limits and by eviction, and by
nothing else. That is the right level for a hobby map of evolving organisms, and it should be
said rather than left to be assumed in either direction.

**It does not deliver a control surface**, if Decision 2 goes the recommended way. The
operator will be able to see everything and change almost nothing on a participant's world.
Risk 9 is the sharpest example and the accepted cost is stated there.

**It does not return a permanently rejected organism.** §13 item 2 stays parked, and the only
change is that the operator it is held for may now be somebody the owner has never met.

**It does not fix the game.** `SpeciesHistoryGuard.cs` guards one real game-side defect that
the multiverse's own traffic provokes, and that guard ships with the package. Other defects
that traffic provokes will be found by strangers running the mod, and finding them is a
legitimate output of the playtest rather than a failure of it.

**It does not do direct peer-to-peer transport.** That is M6: libp2p behind unchanged Contract
B shapes, NAT traversal, peer discovery, gossip, lease-based slot claims, with the relay
degrading to bootstrap and relay-of-last-resort. M5's credential design should be read with M6
in view, because a per-peer secret issued by a relay is a different object from a peer
identity that survives the relay's demotion.

**It does not do the species catalog, corpses, pellets or eggs.** That is M7, and D6's
graduation question is the one part of it Decision 3 may pull forward.

**One numbering caveat, because the tree still contradicts itself.** D16 renumbered the
milestones on 2026-08-05 and corrected `system_decomposition.md` throughout; the older
documents were not corrected. `m1_considerations.md`, `m2_considerations.md` and
`m2_findings.md` say "M5" where they mean **M7**; `m3_considerations.md` says "M5" where it
means **M6**; `contract-b-m2.md` carries a stale renumbering of its own. Contract Change 16
fixes them. Until it lands, a reader who finds "M5" in a document written before 2026-08-05
should assume it does not mean this milestone. The `[M5-SPECIES]`, `[M5-CENSUS]` and
`[M5-HISTORY]` log prefixes in the mod are a separate thing entirely and are not to-do
markers: they name the milestone *window* in which those series landed — after the M4 exit
test and before M5 — and the work they belong to is already in the living deployment.

## Next Steps

1. **Get Decisions 1, 2 and 9 ratified.** WP1 cannot start without the first, and the other
   two change what WP1 writes.
2. **Write `m4_findings.md`.** It is M4's one open deliverable and it is also the baseline a
   public map will be compared against. It gets harder to write every week the rig runs.
3. **Keep the living deployment running, and treat it as M5's first participant.** Every
   upgrade the milestone ships has to move it, and it is the only peer whose behaviour before
   and after is fully observable.
4. **Run `phase5far` while there is still one far end that answers the phone.** After M5 every
   peer is a peer the rig cannot drive.
5. **Build the churn harness before the people.** WP5's synthetic map is what stops the
   playtest from being the first time auto-placement meets a stranger.
6. **Do the cheap contract closures in the same wave** — items 11, 12 and 13. They are
   additive today and a migration story afterwards.
