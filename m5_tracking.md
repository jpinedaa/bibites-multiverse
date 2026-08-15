# M5 Implementation Tracking

**Status, 2026-08-11 end of day: WP1 done; WP2 done (the fleet crossed, and slot 6 crossed
the same evening — the whole map now on `contract-b/4.0`); WP4 and WP5 implemented AND rolled out with no map outage;
WP7's spine drafted with its WP2/WP4 slots closed. Remaining: WP3 (opens on the owner's hosting
calls), WP6 (packaging — startable now that WP4/WP5 are live), WP7's closing arcs, WP8. The
live map runs limits, placement and the coalescing window; the archive-restart debt (deny list
+ `limits` visibility on the status page) is banked for the next batched archive reason.**

This is the orchestrator's execution state for M5. `m5_considerations.md` is the milestone's
design and it is ratified; this document is what is *done* against it, what is next, and the
rules the doing has to follow. The two are meant to be read together and never to say the same
thing twice.

## What this document is, and how to use it

**Read this, then read the sections it points at.** A session that opens M5 needs four things
before it dispatches anything: the decided state (below, compressed), the work-package order,
the operating rules this project runs on, and the constraints the living deployment puts on
every rollout. All four are here. Everything else — why a decision went the way it did, what a
package must contain, what the exit test asks — stays in `m5_considerations.md`, and this
document points rather than restates.

**The division of labour, stated so it survives:**

| Question | Where the answer lives |
|---|---|
| Why is the wire going to `contract-b/4`? What must WP3 contain? What does the exit test require? | `m5_considerations.md` — decisions, work packages, exit test |
| What is decided, project-wide, and what does it bind? | `system_decomposition.md` — D1–D25 and the milestone entries |
| What does the rig do when you deploy to it? | `dev_environment.md` — *The living deployment*, *Watch items*, *Gotchas* |
| Is WP4 started? What did it land? What is safe to start next? | **here** |

**This is a living document, and it is updated by integration rather than by appending.** When a
package lands, its row changes state and its evidence goes with it; when a package changes
shape, the row is rewritten rather than annotated with a correction underneath the old text. A
finding that contradicts something already written here is surfaced and resolved, not stacked
beside it. If an update would make a section too long or land in the wrong place, restructure
the section. The same rule the project applies to every other document applies to this one, and
the reason is the same: the next reader is a fresh session with no memory of the edit.

**One thing this document must never become** is a second copy of the design. If a status line
starts explaining *why*, the explanation belongs in `m5_considerations.md` and the line belongs
here as a pointer to it.

## The decided state, compressed

Nine decisions were ratified on 2026-08-10, and two of them were refined by the owner on
2026-08-11. Five became D21–D25 in `system_decomposition.md`; four are milestone-internal and
live in `m5_considerations.md`, *Decisions for the Owner*. One line each — the argument is at
the pointer.

| What is settled | The call | Pointer |
|---|---|---|
| **D21** — the wire's version | Per-peer credentials and TLS ship **together** at **`contract-b/4`**, the credential bound to the `peerId`; Contract A's bearer token is the minor **`contract-a/2.4`**. Taken in WP1, pre-strangers, for D13's reason | DQ1, decision 1 |
| **D22** — compatibility | **Layered.** The relay's membership test is the **contract** version; the game version is a per-machine matter answered by a published support matrix. Refined 2026-08-11: the bb8 payload stays opaque, cross-version loading is **assumed**, the envelope's game version is **diagnostic metadata** no *new* reader may parse into a capability decision | DQ5, decision 5 |
| **D22, second call** — the shipped gates | **They stay.** `VERSION_UNSUPPORTED`, `mapwalk`'s `peer_incompatible`, close `4002` and §6.1's relay refusal are all kept and written down as named exceptions. **WP1 retires nothing.** The accepted behaviour is a **partition** along a version boundary after a staggered game update; the whole cross-version design space waits for that to become an operational problem | DQ5, *ANSWERED 2026-08-11* |
| **D23** — the control surface | **Deferred to M6.** The operator surface stays read-only through the public release, and Risk 9 is the accepted cost, named and dated | Decision 2, Risk 9 |
| **D24** — the hosting commitment | A **bounded, announced** run. The period is stated to participants before they join, and the ending is a stated event with a wind-down procedure that says what becomes of `ring.json` and the archive's durable files. **Period chosen by the owner 2026-08-11: 3 months** — extensions may be announced later, the bound is never silently shortened. Hosting substrate under iteration (AWS vs GCP, spot-simulation idea) — `wp3_hosting_options.md` | Decision 4, DQ2 |
| **D25** — the channel | **GitHub Releases**, with published checksums and the `Unblock-File` story on the release page itself. It pushes nothing, so the fleet moves by publication plus a relay-side minimum contract version. It also carries decision 7's stated default: **D17's four edges, client on**, stated in the packaging documentation and the installer's own output | Decision 6, decision 7, DQ4 |
| Archive retention (milestone-internal) | **ANSWERED by the owner 2026-08-12: the ledger is kept forever; genome blobs are pruned to a horizon** (default 30 days, a knob). Risk 7 becomes policy: an unfetched genome past the horizon is permanently unfetchable, and the eviction feature (nothing evicts today) is a new archive arc. Bundle plan: start on the $12 Lightsail (trial-covered), resize on the monitor's RAM tripwire — the kept-forever ledger's memory still grows (~184 B/record replay, ~165 resident). **Corrected 2026-08-14**: day 45 was the streamed *replay* peak crossing 2 GB, but since the replay was streamed the **resident** set is the larger term and `monitor.sh` alerts on the larger — `deploy/SIZING.md` §4 puts the 2 GB resident crossing at **~day 28** and the tripwire (0.85 of a ~1.9 GiB `MemTotal`) at **WARN ~day 18, CRIT ~day 24**. The resize still converts that from an outage into a bill, but it lands in week three, not week seven | Decision 3, DQ3, Risk 5, Risk 7 |
| The exit bar (milestone-internal) | **≥4 non-owner peers, ≥72 continuous hours, zero operator actions on any participant's machine.** Relay-side actions are allowed and counted | Decision 8, *Exit Test* |
| The velocity floor (milestone-internal) | **Not built.** Instrumented during the playtest, and the measurement decides whether it is ever built. Contract Change 14 leaves the milestone; Risk 8 is the accepted cost for the duration | Decision 9, Risk 8 |

**Nothing in M5 is gated on a signature any more.** Every package below is startable in
dependency order.

## Work packages

Eight packages, as `m5_considerations.md`, *Work Packages* defines them. **Delivers** is one
line and is not the package's contents; **Done when** is the bar the package reports against.
Neither replaces reading the package.

| WP | Delivers | Depends on | Status | Done when |
|---|---|---|---|---|
| **WP1** — The contract amendments | The wire M5 ships: `contract-b/4`, `contract-a/2.4`, and the sixteen M5 rows of *Contract Changes Needed* | Nothing — its gate opened 2026-08-10 | **Done 2026-08-11.** Commit 35f208f landed item 16; commit 350988e landed the wave: `contract-b/4.0` as `contract-b-m4.md` §22 (B22–B32, in place, path `/contract-b/v4`), `contract-a/2.4` as `contract-a.md` §21 (A47–A52, path unchanged), `genome-hash.md` re-verified at `bb8-genome/1`. Every done-when clause met: row 13 assessed (A51 — stays open and moot), row 14 not written, D22's statement in B31 + A48 with the four gates as named exceptions, migration note in B32/A52. The two sets cross-cite by §- and amendment number | Both contracts published at their new versions; every M5 row applied, or assessed and recorded as closed (row 13); one worked example per changed or added message; D22's layered statement written with row 10a's diagnostic-only rule binding **new readers only** and the four shipped gates named as kept exceptions; the migration note for the living deployment written; item 16's stale-milestone corrections landed. Row 14 is **not** written |
| **WP2** — Transport security | TLS on Contract B, per-peer credentials bound to `peerId`, the archive as an authorised subscriber, a bearer token on Contract A | WP1 | **Go side done 2026-08-11** (commit 0393698): B22 credentials + join strings (`peercred`, verifier store in `<data-dir>/peers.json`), B23 TLS with rotation-surviving reload and the off-loopback 426 rule, B25 version floor (default unset), B27 subscribe grant with the boundary tested as not-a-privileged-view, B32 path move with v2/v3 retired, A47 sidecar side (`modtoken`); Risk 1's adversarial test passes both halves (`TestRisk1_ValidCredentialWithAnotherPeersID`). **Mod side done** (dc9d01f: 0.6.4 sends the bearer token at `contract-a/2.4`; 53c2f1f batches A49's flag and A50's 4007 handling into the same undeployed build). **Local crossing DONE 2026-08-11 17:20Z** (window executed per `e2e/crossing/RUNBOOK.md`; commits 8e714ba, 32f41f8): 7 m 18 s map-down, five slots back at mod 0.6.4 / `contract-a/2.4` on `wss://…/contract-b/v4`, **zero** discarded journal bytes on all five, **zero** ledger gap (archive restarted inside the outage; re-subscribed 880 ms before the first sidecar), no custody burst (peak 4 vs the mod-deploy cycle's 55 — `down` takes the sidecars too, so journals replay flush). **Far-end crossing DONE 2026-08-12 ~01:19Z** (far end / slot 6, executed on the evening of the 11th): CA trusted into the far end's `Cert:\CurrentUser\Root` (thumbprint `C465F51E…094E`, `-SkipCaImport` for the non-interactive steps then the one human-gated import), slot 6 granted `reason=reclaimed` at (2,1) on `wss://…/contract-b/v4` (relay `m5.0`), mod **0.6.4** / `contract-a/2.4`, all four edges `peer_live`, zero heartbeat timeouts, and it held its reservation throughout so no `holdTimeoutMs` bounce fired. Its sidecar was then brought current to the WP4/WP5 build (`4dd2ac1`) at ~01:28Z — mod DLL unchanged at 0.6.4, so a sidecar-only swap | The functional path works **and Risk 1's adversarial test passes**: a peer presenting a valid credential with another peer's `peerId` is refused at the handshake and the legitimate peer observes nothing. The archive's visibility boundary is stated, not implied. The living deployment has crossed to `contract-b/4` in lockstep with zero discarded journal bytes |
| **WP3** — The hosted deployment | The relay and archive as a *service*: VPS, DNS, ACME, supervision, monitoring, backup, restart policy — plus the forward receipt, D24's commitment and the retention rule | WP2 | **Options costed 2026-08-11** (`wp3_hosting_options.md`); **three picks made by the owner 2026-08-12: AWS (Lightsail), an owner-registered domain (name itself still to be chosen — it is permanent once the first join string is minted), and the relay on port 443** (status page keeps its own port). **Owner 2026-08-12: no developer ask — everything approved to proceed**, including the Stage 2 cloud-world rehearsal. **Same day: the project will support the game's native Linux build** (itch.io) — **and the rehearsal answered GO** (e00b298): the itch build IS 0.6.3.1, all six patched types byte-identical to Windows (the 512-byte delta is a file-dialog shim), the mod loads under BepInEx linux_x64 and ran a 14-minute authenticated session headless; Proton/Wine drops out of the plan. New matrix row: `0.6.3.1 / Linux → 5B145A0A…` with a schema change (platform/store columns) owed to `docs/support-matrix.md`. **WP6 now owes the ten Linux items in the rehearsal report** (Linux installer, linux_x64 BepInEx artifact, run_bepinex.sh launcher docs, save-path + XDG relocation, the shared-LogOutput multi-instance warning — the Windows trap's inverted twin, louder because the mod works while the logs lie). Cloud sizing improved: Linux game 368–401 MB / ~1 core, sidecar 13 MB; Stage 2's first job = the achieved-speed measurement on a clean box (this host was too loaded to settle it). Rehearsal assets **survived the 2026-08-12 volume reorganisation and are at `/mnt/wsl/data/scratch/m5-linux-rehearsal/`** (274 MB; the game dir is still install-ready for Stage 2, and `release/make-release.sh` and `release/test-install-uninstall.sh` both default to reading it — verified 2026-08-13). **They sit on the `scratch/` tier, which is named for what gets swept**, so a Stage 2 that needs them should either move them to `archives/` or accept the re-do: the 79.8 MB Linux build must be fetched again from itch.io **with the owner's account**, because those download links are per-purchase and time-limited (`docs/support-matrix.md`), then unpacked with BepInEx `linux_x64` as the rehearsal did. **The name is chosen 2026-08-14: `bibitesmultiverse.com`** (verified available at Verisign RDAP that day; `MV_DOMAIN` and `MV_CERT_EXTRA_NAMES=status.bibitesmultiverse.com` are set in `deploy/deploy.env.example`; the registrar comparison, the RDAP method and the shortlist the name came from are in `wp3_hosting_options.md`, *Which registrar to buy from, and which name to buy* — Cloudflare Registrar ~$10.44/yr flat, Porkbun the fallback) — the owner registers it; it is permanent once the first join string is minted. Still open: which Lightsail bundle. **Built 2026-08-12**: the hosting kit (`deploy/`, 2a96a9e — provisioning, systemd, ACME on 443 with rotation verified live, ntfy monitoring with a dead-man's heartbeat, backups, SIZING/RESTART-POLICY/WIND-DOWN/ANNOUNCEMENT; join-string minting needs a batched relay restart — B28 has no mint act, the amendment to write if onboarding at rate ever hurts) and B26's forward receipt (76318f9 — 287 B/receipt, 1.8% of migration egress, receipts enter no published limit; rollout order sidecars-then-relay). Status-page gzip implemented (dab524d). **Retention answered and built 2026-08-12**: the ledger forever, genome blobs pruned to a 720 h horizon (contract §23 B33–B34, 2b238d4 — no version bump; §6.10 already defined the pruned-hash answer); the eviction pass + gap retirement and the streamed replay landed together (c4543b6 — replay peak 184 B/record vs 1,286, `ledgerRecords` drift fixed; kit re-verdicted: GOMEMLIMIT=5GiB ceiling, swap 0, tripwire on the larger of replay/resident). **Debt window executed 2026-08-12 11:18–11:46Z** (e01f776): sidecars paused 5m21s → archive restarted inside the pause (**zero ledger gap, empty by construction**; streamed replay's first live run **1.53 GiB peak vs 7.86**, 148 s, counter reconciled to 0) → relay (receipts live — journals show them climbing on all five) → sidecars rolled (zero discarded everywhere). gzip live (9.15×), horizon proven off-by-omission. **Two corrections**: the `limits` key on `/api/status` was NEVER implemented (WP4 built the wire half only — small arc dispatched; rides the next archive restart / the VPS provisioning); slot 6's per-migration WARN stream was unbounded (~23 MB/day) — **RESOLVED: the far-end trip happened 2026-08-12** (a665042, recorded in `dev_environment.md`): slot 6 runs the HEAD sidecar (pacing + `--diagnose` + `--my-slot` + B26) and the warning stream is over. **Remaining**: the domain name; the owner's 12 manual steps; then provisioning and the announcement | The service restarts on exit and on boot; a certificate rotation is survived rather than scheduled around; monitoring reaches a person for peers lost, persistent bypasses, free disk and error lines; `ring.json` and the archive's three durable files are backed up; the restart policy and its participant-facing statement are written; the forward receipt ships **with its measured per-migration cost at rate**; D24's period is announced and its wind-down written; the retention rule exists with the per-peer growth arithmetic beside it |
| **WP4** — Capacity, abuse limits, admin path | The published limit table, the authenticated admin path, the render deny list, and the three cheap contract closures | WP1, WP2 | **Implemented and rolled out 2026-08-11** (dfbd1dc): B24's eight limits as knobs, frame-level only (D1 guard test), published on both frames; B28's admin path as a separate listener over the admin grant (two-call confirm, ring-state hash, audit line; eviction deliberately shapeless — `4005`, pinned by test); B30's deny list + the markup-species-name CI test; A49/A50 sidecar halves (ledger now tells `blob_dropped_for_size` from `parent_gone`). Mod halves shipped earlier in the 0.6.4 batch (53c2f1f); closure 13 was WP1's (A51). **Rolled out to the living deployment 2026-08-11**, batched with WP5 and costing **no map outage** (*`dev_environment.md`, The living deployment → The WP4 + WP5 rollout*): relay restarted 20:56:15Z with the limit envelope on its startup line, five sidecars rolled one at a time 20:59:10Z–21:03:53Z, every one `reason=reclaimed` on its own coordinate with **zero discarded journal bytes**, `heldDepth`/`timeoutBounces` 0 throughout, A50's partial-case WARN silent as a full 3×2 predicts. `--admin-listen` was deliberately **not** passed, so B28's listener is compiled in and bound to nothing until an owner-level act turns it on. **The archive restart happened** (the 2026-08-12 debt window) — the deny-list capability shipped, deliberately unset on the rig. **The `limits` key on `/api/status` turned out never implemented** (this package built the wire half only; the window's verification caught it) — **closed 2026-08-12** (e6d7911): `limits` + `minContractVersion` ride the view key-for-key with the two absences kept distinct (pre-B24 relay = UNKNOWN, absent floor = "no minimum"), the settings tab gains the map's own card, ringstat prints the same values. Rides the next archive restart / the VPS provisioning; until then the published table is readable only in the relay's own log | Every limit is a knob, every peer-visible one is published on the stats block, and **every one is countable at the frame level** — D1 survives the package. Release, handover and eviction run over an authenticated path and keep the printed held-entry report. Contract Changes 11, 12 and 13 are closed |
| **WP5** — Placement and route-around under churn | Auto-placement, the coalescing window, the slot-space policy — proven against a synthetic churn harness | WP1, WP2 | **Implemented and rolled out 2026-08-11** (9d3e388): B29's holes-before-growth narrowing (rig's 3×2 layout pinned claim-for-claim), the widening coalescing window (250→2000 ms, threshold 8), the quiet re-claim, the slot-space policy rendered identically at both doors. **Harness run to exhaustion: 50 M events in 5 m 42 s, seven invariants clean, `maxSlotEverIssued` exactly = placements (48.3 M returns cost zero addresses), no address re-issued, broadcasts 4 rounds per ~1,700 changes vs bound 22.** **Rolled out 2026-08-11** in the same relay restart as WP4 (20:56:15Z), relay-only as planned. Measured on the living deployment: `PEER_STATUS` **55.4/min → 43.9/min (−20.6%)**, and `maxSlotEverIssued=6` on a six-slot map read straight off the new slot-space startup line. **That reading is WP5's floor, not its measure** — DQ3's 64-re-claims-a-day peer *is* slot 6, slot 6 is dark, and the five local slots claim a handful of times an hour; what remains is the §14 B4 stats term the amendment does not bound. Re-measure when the far end rejoins. Two contract findings held for the owner: the rectangle never shrinks (skip lists grow with map *history* — Risk 4's neighbour, pinned by test) and B29's `borderEdges` is compared but never published (§6.3.1 gap) | Holes fill before an axis extends, and the axis rule is stated; the coalescing window and the `PEER_STATUS` broadcast bound are measured; `maxSlotEverIssued` growth is **tested rather than assumed**; the operator's answer for a position that will never be filled again is written; and the churn harness — join, leave, return, never return — has been **run to exhaustion before WP8 involves anybody** |
| **WP6** — The package | A GitHub release a stranger can install from, with the defaults audit and the support matrix | WP2 (input: `farend/`) — **needs the game** | **Built 2026-08-12** (20f1414): `release/` — installer/uninstaller (uninstall proven, 61 checks over 5 scenarios), `make-release.sh` (clean-tree rebuild, publishes nothing), release-page text with the checksum-then-`Unblock-File` order, `docs/support-matrix.md` + machine-readable JSON the installer enforces, `docs/defaults-audit.md` (2 pass, 1 packaging fix, 1 WP3 finding, 1 code-level log-severity finding). **Publish pending, owner's hands**: 4 steps in `release/README.md`, gated on WP3 supplying D24's period text and the repo being public. **Linux extension landed 2026-08-12** (4777ac2): two release archives, the (version, platform)-keyed matrix at schema /2, the Linux installer/uninstaller with a 97-check proof (which found and fixed a real BepInEx `changelog.txt` clobber; the Windows `Expand-Archive -Force` twin is latent-unreachable and flagged, not fixed blind), `LOCAL-LOGSHRED` in the taxonomy | An installer with no build toolchain and no instruction to disable a security control; the release page carries artifacts, published checksums and the `Unblock-File` story before the download; D17's default is stated in the documentation and in the installer's output; the four-default audit is complete (`--insecure-no-token`, the archive bind habit, the exclusion default, the save defaults); D22's support matrix is published beside the release and the installer's per-machine check enforces the entry; the uninstall leaves the player's game as it found it |
| **WP7** — The support surface | The error taxonomy, `--diagnose`, the participant's view of their own slot, renderer escaping | WP1 — **needs the game** for the mod-side failure modes | **Spine drafted + WP2/WP4 slots closed 2026-08-11** (f4bfd2e spine, 61d905b closures): taxonomy quotes the shipped refusal lines and limit defaults; evicted-peer entry rewritten as designed behaviour; renderer escaping landed WITH its CI test in WP4 (dfbd1dc, `TestAMarkupSpeciesNameIsNeverRenderedAsMarkup`). **Done 2026-08-12** (385991f implementation; 6e7c57d roll): `--diagnose` (21 checks read-only, exit 0/1/2, `multiverse-diagnose/1` JSON) and `--my-slot` (loopback own-slot view, no wire message, no token) — rolled onto all five local sidecars with zero discarded bytes and live-verified: 13 old-build UNKNOWNs collapse to 3 honest gaps, exit 0 under the rig's real flags, non-disturbance proven. WARN band closed at 0.75 of a published ceiling. **The package's own bar is met**; its remaining doc slots are owed BY WP3 (addresses, period) and WP8 (the two measured bands), not by this package. Optional follow-up: a `Diagnose-Multiverse.ps1` kit wrapper | Every refusal in the taxonomy names the remedy **and who must act**; `multiverse-sidecar --diagnose` runs the checks the runbook does by hand; a participant can read their own slot's liveness, lanes, paced depth and last save without an operator; renderer escaping has the markup-species-name test **in CI, not by eye**; the documentation covers install, join, diagnose and leave |
| **WP8** — The playtest | The exit test, the two instrumented watches, and `m5_findings.md` | WP2–WP7 — **needs the game on other people's machines** | **Not started** | The eight parts of *Exit Test* run against the ratified bar; the velocity-floor signature and the version-skew **partition** are counted rather than eyeballed; the payload assumption is reported as **untested**, never as stable; the record — per-peer archive growth, crossing rates, epoch rate, broadcast volume, disk arithmetic — is captured; and `m5_findings.md` is written. M3 and M4 both left this deliverable open |

### Recommended sequence

**1. WP1 alone.** *"Write WP1 before any code. M2 and M3 both paid for the alternative."* It is
the root dependency: `contract-b/4` is the wire every other package builds on, and taking the
major before strangers run the build is the whole of D21's reasoning. Nothing else should start
while the wire is being written, because everything else would be built against a moving target.

**2. WP2 next, with WP7's documentation spine beside it.** WP2 gates four of the remaining six
packages. **WP7 is the only package that does not depend on WP2** — it depends on WP1 alone — so
its taxonomy skeleton, its documentation set and `--diagnose`'s check list can be drafted in
parallel. It cannot *finish* there: half the refusals the taxonomy has to cover are the ones
WP2 invents, so WP7 closes after WP2 lands.

**3. After WP2 — WP3 in parallel with WP4, then WP5.** WP3, WP4 and WP5 declare no dependency on
each other, but they are not equally parallel in practice. WP3 is off-rig work (a VPS, DNS,
supervision), so it genuinely runs concurrently with anything. WP4 and WP5 both change relay
behaviour and both want the living deployment to verify against, so **serialise those two
against each other** rather than dispatching both at once — the rig is one map and two agents
restarting the same relay is how a rollout becomes an incident. WP4 first is the cheaper order:
its three contract closures (11, 12, 13) are additive and are the last moment they are cheap.

**4. WP6 after WP4 and WP5 land.** The plan only requires WP2, and that is a judgment call this
document is making rather than a constraint `m5_considerations.md` states: the package has to
carry the final defaults, the published limit table and the support matrix, so packaging before
WP4 and WP5 have settled the sidecar and the relay means packaging twice. It also needs the
game, which makes it a rollout rather than an edit.

**5. WP7 closes before WP8.** The taxonomy has to cover every refusal WP2 and WP4 added, or the
playtest discovers the gaps on strangers.

**6. WP8 last, and only after the harness and the abuse cases have been rehearsed.** Risk 2's
accepted cost is that the exit test **cannot be re-run cheaply** — it spends other people's
goodwill each time — which is exactly why WP5's churn harness runs to exhaustion first and WP4's
abuse cases are rehearsed against the rig before Part 5 is run against a live public relay.

**One rollout fact that shapes the middle of this sequence.** WP2 puts a bearer token on
Contract A, which is a **mod** change, so the rig's crossing to `contract-b/4` is a five-games-down
deploy and not a rolling sidecar change. Batch every other pending mod change into that same
window — the constraints below price a mod deploy at about 100 seconds of downtime plus a custody
burst, and each one also costs the far end a bundle rebuild.

### Carried actions that belong to no package

- **The genealogy view: LIVE 2026-08-12 22:03Z** (built 5f2dc83; third zero-gap window of the
  day, e5ff428 — 9.51 M records replayed at 41.2 k/s, VmHWM 1.84 GiB). The live sweep verified
  the merged view end to end (mini-maps cross-checked 4 ways, zero console errors) and found
  **three rendering defects, fix in flight batched with the seed-stock hiding**: the root badges
  clip unreadably out of the label column (bdb5efb is live-but-invisible), the ancestry-floor
  boundary never draws (`>` where `>=` — equal is not strictly inside), and the plot is fixed
  1344 px (now-edge off-screen below 1427 px windows). **All three defects + the seed hiding verified live 2026-08-12 22:54Z**
  (979d72b landed; 0ff9873 the fourth zero-gap window) — badges readable, floor drawn, plot
  elastic, seed hidden with a free reveal, brain ring stabilized (latest-READ genome, one
  membership set across ten polls). Replay-rate reading CORRECTED: 57 → 47.8 → 41.2 → 47.3 k/s
  — the series tracks host load too; size pauses from ~45 k/s and treat it as noisy. **The axis call was ANSWERED by the owner
  2026-08-13** — the drawing fits the living record (2b1e705): the clamp is gone, the axis starts
  at the oldest drawn bar, the floor became a left-margin caption keeping its stat line and root
  badge, and the dashed boundary draws only where the floor truly falls inside the drawn span.
  **Same commit made the ancestry legible** (owner: "hard to tell how the tree is"): the label
  column steps by depth with `├─`-style brackets joining parent to child, hover/tab lights a
  species' whole lineage and dims the rest, the parent drop elbows into the child's bar, the row
  tooltip names the parent unopened, and three new glossary terms explain the drawing to a
  first-time reader (`timeaxis`, `descends`, `lineage`).
  **The view is FINISHED and verified live 2026-08-13** across two windows (652debf, c7692b7):
  the axis measures **0.00% empty** against 58.1% on the same map minutes earlier — and the two
  pre-fix readings proved to be the *same* 3.83-day gap, so "the gap widens" was right about
  duration and wrong about the ratio. The one defect the first sweep found (keyboard focus dying
  on the ~2 s repaint, taking the lit lineage with it) is fixed and soaked: a tabbed row held
  focus and its family across **18 consecutive repaints**, row-to-row never blinked dark across
  2,035 samples. Honest residual: 12 of 18 bars still start within 4 px of `now` (that is what
  the record holds on a fast-turnover map, not a drawing fault) and the horizontal scale now
  re-fits between polls. Process lessons recorded: force a cache-bypassing reload when verifying
  `page.go`; use `file_mark`, never byte offsets, for rig waits; log rotation invalidates a byte
  mark. One judgement to know: the page's gzip-floor test was relaxed 3× → 2× rather than
  trimming explanatory prose to pass — the property guarded (a body that stopped compressing)
  lands at ≤1.0×.
  **Two further owner observations closed 2026-08-13** (7fbaaa6, verified live c003e5c):
  collapsed runs now sit ON the time axis across the interval they actually held the line
  (measured 308.6 px against 308.629 expected for a 40.055 h run — the 40 h of empty plot IS the
  mark now), and a child recorded before its parent (`Sheeplasius bananenbrotus`, 2h21m) draws
  backwards in amber from the parent's bar start instead of an impossible geometry. The same
  commit fitted the brain ring to the drawn set: **226× better discrimination** on today's data
  (4.000 px spread against 0.0177), real counts on the ring's own tooltip, re-fit caveat written
  down. **The standing residual — the ring depended on archive process uptime — is CLOSED by the
  brain-history work below**, which persists each species' last measured brain instead of caching
  it per process.
- **The brain-complexity panel (owner-requested 2026-08-13, DONE and verified 2026-08-14).** A
  panel under the genealogy on the genealogy's own clock: median synapses and median hidden
  neurons per genome with an interquartile band and a coverage strip (86aa7a4, panel defects
  fixed 10919b8, verified 9184eae/98592cb). The measurement folds at **genome arrival** — where
  the bytes are already in hand and the crossing that wanted them is known — never by sweeping
  the store, which would have refreshed every blob's mtime and postponed the retention horizon
  for the whole archive. It is **persisted** (`<data-dir>/brains.jsonl`, ~6.9 MB/day,
  append-and-compact, header unrewritten across three restarts) and replayed, not recomputed.
  **The owner's second insight — persist per-species too — closed a standing defect**: an
  extinct ancestor now keeps the last brain the archive ever read of it, so rings survive both
  the blob's pruning and a restart (2 of 14 rows ringed after a restart → 13 of 17 in ten
  minutes). Wording follows: "ever read", with the measurement's age. **What the data says:
  median synapses 8 → 42 over the week (×5.1), synapses per neuron 0.16 → 0.74, median hidden
  neurons 1 → 8.6 (×8.6) — Spearman 0.971 against time.** Raw neuron count understates ×7
  because 48 neurons are a fixed floor. Coverage is 81% and the missingness is species- and
  content-blind, which is what makes the sample defensible; the 45 hours the rig was down draw
  as gaps.
- **The genealogy view's FINAL SHAPE, decided by the owner 2026-08-12 (build was dispatched on this):** the
  stratigraphic lifespan tree — time on the horizontal axis, species as first-seen→last-seen
  bars — **replacing both the species tab and the tree tab with one merged view**. Included:
  per-leaf 3×2 mini-maps, population sparklines (from `/api/species/history`), collapsed chains
  as dotted lines with length ∝ generations, brain-complexity styling from each species' latest
  stored genome (bb8-parsed, cached per hash), the glyph as the sole colour carrier (no
  redundant swatches), the root/floor badges carried over. **Rejected: crossing pulses,
  map↔tree cross-links, time scrubber, SVG export, radial layout.**
- **The genealogy tree (owner-requested, 2026-08-12, built — 41eef26).** `/api/species/tree` +
  a page tab: the living species as leaves, ancestors reduced to branching points, chains
  collapsed. Derived entirely from A30/B9's parent-species field the ledger already carried —
  no wire change. On the live rig: 11 of 12 living species connectable, deepest lineage 40
  generations, 2,407 recorded species reduce to a 15-node tree. **Live on the map since 2026-08-12 16:26Z** (the
  archive-views window, f983213 — same zero-gap pattern, archive-only, 4m57s pause; the limits
  view went live in the same restart). Served shape at go-live: 18 nodes, 2 roots, 14 living
  leaves. Replay note: 47.8 k rec/s vs the prior 57 k — the tree's per-record maintenance is the
  prime suspect; size future paused windows from 48 k/s. **The record's floor, investigated
  2026-08-12** (the owner asked why Basic bibite is not the ultimate ancestor): all life on the
  map genuinely descends from the seed stock, but the first 19.5 h / 203,168 crossings predate
  A30's species blocks, and nameless arrivals founded 408 permanently parentless species — the
  living family's true recorded top is `Livilus yodexxus`, extinct since 08-07 and so forever
  unlinkable. No fabricated edges; the root-badge fix **landed** (bdb5efb): a root the
  record reaches above wears "THE RECORD BEGINS HERE · N GENERATIONS ABOVE" distinct from the
  amber no-ancestry badge, and the tab prints the record's floor ("ancestry recorded since
  2026-08-07 UTC") from one maintained int64. Rides the next archive restart. The 143
  conflicting parents (last-write-wins by `recordedAt`; a deep chain can shift between polls)
  stay a WATCH NOTE — the aggregate does not count overwrites and a counter for a caption was
  refused.
- **Brain complexity over time, and brains that outlive their genomes (owner-requested,
  2026-08-13, built).** A panel BELOW the genealogy sharing its exact axis — same left and right
  edges, same ticks, its own two y scales — drawing **median synapses per genome** and **median
  hidden neurons** (the count above the fixed 48 every bibite brain is born with) with an
  interquartile band and a coverage strip. New endpoint `/api/species/brains?from=&to=&buckets=`;
  `/api/status` untouched, per the metrics rule.
  **The measurement is MAINTAINED, not computed on demand**, and every part of that is forced:
  a bucket is only ~42 % covered while it is new and ~97 % five days later (the fetch backlog
  draining backwards), so the fold happens at GENOME ARRIVAL — `onGenomeResponse`, bytes already
  in memory, keyed on the pending entry's own `crossedAt` — plus `onMigration` for hashes the
  store already holds. There is deliberately **no full-history sweep**: `Store.Get` refreshes
  mtime and eviction is ordered by mtime, so reading all 702 k blobs would postpone the 720 h
  horizon for the whole store and hold its mutex for ~11 minutes. The aggregate is **persisted
  and replayed** in `brains.jsonl` (append-and-compact, 5-minute buckets, ~111 B a bucket
  measured) rather than re-derived at startup, which would add ~11 min to a 28 s replay; the
  coverage denominator is NOT persisted — it comes from the ledger replay, which costs a measured
  **1.47 s over 4.98 M records**. Loss of the sidecar is **"history starts now", never zeroes**.
  **The same fold closes the ring's uptime residual above.** Each species' last measured brain is
  persisted with the crossing it was read from, so an EXTINCT ancestor keeps the last brain this
  archive ever saw of it — forever, past the retention horizon that deletes the genome — and a
  restart no longer empties the picture. The ring's words changed with it: "the newest genome of
  it this archive **ever read**", with the measurement's own age on the row and in the tooltip,
  because an ancestor's reading can be days old beside a living species' current one. Absence is
  still absence: a species never read draws no ring.
  **Rejected:** a per-species brain overlay across the whole axis (empty before 08-07 05Z, 4–8
  samples in a dozen buckets); an opacity ramp for coverage (it would fade the newest and
  least-covered stretch, which is the part a reader looks at hardest).
  On real rig blobs through the real pipeline: median synapses **4 → 45** and median hidden
  neurons **0 → 10** across the record, with the two known dips reproduced and the record's
  43 h of outages drawn as breaks in the line.

- **Keep the living deployment running, and treat it as M5's first participant.** Every upgrade
  the milestone ships has to move it, and it is the only peer whose before and after are fully
  observable. (`m5_considerations.md`, *Next Steps* 3.)
- **Run `phase5far` while there is still one far end that answers the phone.** It has never been
  run; it needs two commands typed at the second computer, and D9 forbids the rig to send them.
  After M5 every peer is a peer the rig cannot drive. (*Next Steps* 4, *Inherited threads*.)
  **2026-08-12: armed and found uncalibratable as written** — its hardcoded §15 A20 pair (burst 5,
  2.0/sim-min) predates both the 2026-08-07 default raise (100/50) and the map's evolved migration
  pressure (slot 6 measures 132.5 inbound/min at 6.5×; ~20/min with every sender at 1×, ten times
  the drain bound). A pin rehearsal on slot 6 dammed 33 in 45 s and was reverted before the
  64-entry queue cap. The far sidecar now paces its own outbound, so the burst-on-rejoin urgency
  behind the thread is gone; what remains is either parametrising the phase's rate/burst/bound and
  running it calibrated, or closing the thread on today's evidence. Details in
  `dev_environment.md` (*The far end took its owed swap*, *phase5far cannot run as written*).

## Orchestration protocol

These are the operating rules the session that produced this document proved. A fresh
orchestrator inherits them whole.

**Opus 5 subagents do all substantive work; the main loop orchestrates, integrates and commits.**
The orchestrator's job is to scope an arc, dispatch it, read the report, integrate the result and
commit it. It does not do the work itself.

**One agent per coherent arc.** An arc is design **plus** implementation **plus** deployment
**plus** verification **plus** the documentation the change owes — not a slice of each handed to
a different agent. A package this size will be several arcs; each arc is one agent, and the
agent owns its arc end to end so that the person who deployed it is the person who verifies it.

**Agents do not commit.** An agent reports the paths it changed and a proposed commit message;
the orchestrator stages and commits. This is not ceremony — parallel sessions share this
checkout, and concurrent staging is how one session's index picks up another's half-finished
work.

**Committing, therefore:**

- `git fetch` before staging, every time. Another session may have moved `origin/main`.
- **Stage explicit paths.** Never `git add -A`, never `git add .`, never `git commit -a`.
- Commit **sequentially**, one arc at a time, so the index is never shared between two writes.
- Message style: **imperative, descriptive, one line of subject**, matching the log — *Ratify the
  nine M5 decisions and mint D21-D25*, *Bound the archive's genome pump in work, burst and
  concurrency*. **No AI attribution, no generated-by footer, no co-author line**, in the message
  or in anything the change touches.
- Group commits where the material separates cleanly: **contracts** in one, **implementation plus
  rig plus documentation** in another, and the **far-end bundle** in its own, since that is a
  binary artifact with its own rebuild rule.

**Documentation is integrated, not appended.** Read the whole target document before changing it;
make targeted edits anchored on current content rather than whole-file writes; merge an addition
into the entry that already covers it instead of adding a second version; surface and resolve a
contradiction rather than leaving both readings standing; and revalidate the inbound and outbound
cross-references after the edit. `dev_environment.md`, `system_decomposition.md` and the contracts
are all documents another session may be editing at the same moment.

**The stop rule.** Anything beyond a documented pattern — an unfamiliar failure, a deployment
step no runbook describes, a rig state that does not match what `dev_environment.md` records —
means **stop, leave the system as found, and report with evidence**. Do not improvise a recovery
on a live deployment. An agent that stops with a clear account of what it saw has done its job;
an agent that guesses has spent the deployment's evidence.

## Live-rig constraints during M5

**The rig keeps running through the whole milestone.** Six worlds, two machines, a relay, an
archive and a collector that has been up since 2026-08-07. It is M5's first participant and the
only fleet that rehearses `contract-b/4`. Every constraint below is on record in
`dev_environment.md` and every one of them binds a rollout.

- **Heavy analysis and test runs go through `nice -n 19`.** `nice -n 19 go test -p 1 ./...` is the
  measured form. Unniced load on this host reproduces the sidecar session storm — **it is on
  record twice**, once as the 18:39Z–18:59Z episode and once as a deliberate reproduction at
  21:25Z the same evening that dropped seven sidecar sessions and three mods. Serialising the
  packages costs about 20 s of wall clock and buys the whole episode back. A churn episode dated
  to one of these is not a deployment fault — but it must be said in the record.
- **Build Go binaries to a scratch directory and `mv` over `bin/`.** Five running sidecars hold
  `bin/sidecar` open, so an in-place `go build -o ../bin/` fails with `ETXTBSY`. The rename is
  atomic and leaves every running process on its old inode until its own restart, which is what a
  rolling change needs. Verify per slot by digest — `sha256sum /proc/<pid>/exe` against
  `sha256sum bin/<name>`, plus `readlink`'s `(deleted)` suffix for a replaced binary; an inode
  comparison cannot work on this host (measured 2026-08-11 — `dev_environment.md` has the why).
- **Sidecar restarts are rolling: one at a time, and the gate is zero discarded journal bytes.**
  The proven sequence per slot is `kill_pid sidecar-<n> -TERM`, wait for the process,
  `start_sidecar <n>`, `wait_healthy`, `wait_grant`. Each sidecar must come back to **its own slot
  and its own coordinate** with `reason=reclaimed`, replay its journal with **zero** discarded
  bytes, and reopen all eight of its lanes. **The map should never leave its full lane count in any
  sample** — 24/24 `peer_live` with six slots live, and **18/20 while slot 6 is dark**, because the
  hole at (2,1) closes slot 3's north and south with `no_peer` and re-pairs four lanes as bypasses.
  Count against what the map is, never against 24. Proven on the living deployment 2026-08-11: five
  rolling restarts, zero discarded bytes on all five, the lane reading identical before and after.
- **A mod deploy takes all five games down at once**, because `deploy.sh` copies into
  `BepInEx/plugins/` and a running game locks the DLL. Budget ~100–110 s end to end: quit five,
  `deploy.sh`, then `start_game` **one at a time**. Expect a **custody replay burst** on the way
  back — `custodyDepth` 55 and `pacedDepth` 14 at its measured peak — draining to 0 in about four
  minutes with nothing lost.
- **Re-send the time scale after every world load, and read readiness off `/api/status`.** The
  first `timescale` after a load answers `Time.timeScale=1.00` and **the reported scale sticks
  there**; a second send twenty seconds later takes. A restarted game can land on a *different*
  BepInEx log file than the one it left, so a line mark taken before the quit is meaningless and a
  `wait_log` against it hangs — `/api/status` is per-slot and rotation-proof.
- **Standing owner grant (2026-08-12): zero-gap archive restarts for page-view verification are
  pre-authorized** — "as soon as you can and as many times as you need." The pattern stays the
  proven one (pause the five local sidecars → restart the archive inside the pause → resume with
  the zero-discarded gate); the grant removes the per-restart ask, not the discipline. Size the
  pause from ~48 k records/s replay.
- **Archive restarts are expensive and permanent — and the cost grows with the ledger.** The
  replay scales with ledger size (~93 s at 3.7 M records on 2026-08-10, **~150 s at 6.24 M on
  2026-08-11**; size any budget from today's ledger, not from a recorded figure), during which
  the status page is dark and the crossings that happen are **never copied to the ledger** — the last measured restart cost **1,940 crossings**,
  absent from the record forever. That is §5.1 working as designed: the traffic is untouched and
  only the record has the gap. **Batch the reasons to restart the archive**; never restart it to
  check something.
- **`ARCHIVE_HTTP=0.0.0.0:8796` must be preserved** on every bring-up
  (`ARCHIVE_HTTP=0.0.0.0:8796 ./e2e/run-m4-lan.sh up`). Without it the archive binds its compiled
  `127.0.0.1:8796` and the second computer's operator loses their only view of the map.
- **The far-end bundle is rebuilt after any mod or sidecar change.** `farend/dist/farend-bundle.zip`
  is **tracked**, and its rule is that it carries what this machine runs: run
  `farend/make-farend-bundle.sh` **after** `bibites-mod/deploy.sh` so the DLL in the zip is the one
  deployed here. Re-taking the bundle is not optional after a `contract-b` change — a stale far-end
  sidecar is what refuses an upgraded neighbour's exports. **The far end applies it at its
  operator's leisure**, and whether it has applied it cannot be determined from here; the one
  observable is a `client gone`/`client connected`/`reason=reclaimed` triple for `peer=slot-6` in
  `relay.log`.
- **Slot 6, the relay and the collector are not the orchestrator's to restart.** Slot 6 is the
  second computer and D9 forbids the rig to drive it. The collector is gitignored, hand-launched
  and relaunched by the owner after every reboot. A relay restart drops every peer's session at
  once and is an owner-level act, not a debugging step.
- **The BepInEx log-file starvation trap, on every five-game bring-up.** BepInEx hands out
  `LogOutput.log` and `.1`–`.4` and then gives up; an instance that gets no log file **does not
  merely lose its log — the mod never loads in it**, and it comes up `modConnected=false` with
  `exportEdges=[]`. The tell is an *absence*: starvation writes nothing at all, where the config
  race writes `configuration failed`. The remedy is to restart **that one instance** through the
  rig's own `start_game <n>`, so the environment matches the other four exactly.
- **Archive the BepInEx logs BEFORE any deploy.** `LogOutput.log` is truncated on launch, so a
  deploy that skips `archive_bepinex_logs` destroys the evidence window it is being deployed to
  measure. **It has happened once** — the `0.6.3` deploy cost roughly 33 saves' worth of history
  between 16:26Z and 17:43Z on 2026-08-10, and the gap is permanent.

## Standing watch items that interact with M5 work

- **The outbound-burst defect: found on slot 6's rejoin, proven fleet-wide, fixed and rolled out
  (2026-08-11/12).** Slot 6's backlog drain tripped `maxFramesPerSecond 50` in a shed/reconnect
  cycle — and the pre-roll log shows it was **never only a rejoin problem**: 19 sheds in 48
  minutes across slot-6 (×11), slot-5 (×7) and slot-1 (×1), every peak exactly 51/s, slot 5
  running the full backoff-ceiling cycle on the live map. The fix (9b1592d): the sidecar paces
  outbound under the published `limits` from `HANDSHAKE_ACK` — half-ceiling rate, quarter-ceiling
  burst, a control reserve — plus bulk gated on the handshake ack (which also closed a §5.2
  empty-session-stamp defect). Rolled onto all five local sidecars 02:20–02:23Z with zero
  discarded bytes and 24/24 lanes throughout. **Amended 2026-08-12 ~11:03Z: two sheds on a PACED
  sidecar (slot 1), both inside the rendered-slot-1 pause-freeze window** — the game froze at
  `timeScale 0` (a drawn-UI pause path, see the rendered-instance gotcha), the backlog built
  behind the stalled mod, and the drain burst read peak 51/s despite the 25/s+12 pacer. Root
  cause of the over-budget burst NOT established (candidates: NACK/bounce storm + control
  traffic compressing into one meter window); zero sheds on slots 2–5 ever, zero on slot 1
  outside that window, nothing lost (bounces went home). Watch: paced ≠ shed-proof when the mod
  stalls. Debt CLEARED 2026-08-12: the far-end trip put slot 6 on the paced sidecar (a665042).
  Log-reading caution: grep for `SHED THIS CONNECTION`, not `4007` (id substrings alias).
- **A RENDERED instance can freeze at `timeScale 0` and never self-recover (2026-08-12).** Slot
  1, drawn for a 20-minute session: the min-FPS servo re-arms as expected (achieved 21.8 → 5.6),
  but ~7 s after the first periodic save the world hit `TimeController.paused` — a drawn-UI
  pause path (`SaveController.LockControls` et al.) that headless instances cannot reach — and
  froze for 7m20s (`pacedDepth` capped, custody 244, duplicate retransmits). The servo cannot
  unpause (`CheckMinFPS` is gated on `!paused`). **Remedy, one command, worked first try:**
  `send <n> timescale 100` routes through `ForcePlay` and clears the pause stack. Intermittent
  (the second save did not reproduce it). Slot 6 is the standing rendered instance — if it ever
  reads `timeScale 0` with static `simulatedTime`, this is the first suspect, and the remedy is
  the far-end operator's to type. Also from the same session: the first-timescale-send trap did
  NOT fire on either launch (fifth and sixth clean readings — the re-send-at-1 remains
  mandatory, the second-send correction increasingly looks historical).

Full readings and their evidence are in `dev_environment.md`, *The living deployment → Watch
items*. What follows is only why each one touches this milestone.

- **The 13-second heartbeat verdict is still accruing.** The raise from 3 500 ms landed
  2026-08-10 and the first reading points the right way without being a rate — 3 saves over the
  retired deadline, 0 sessions lost to them. WP2 and WP6 both ship into this: the timeout is a
  sidecar-side number a stranger will inherit.
- **The species-history serializer is the highest-value save-stall lever, and it is undesigned.**
  `lineageMs` is **53% of `writeMs`** and flat against population, and the term compounds because
  brains only get bigger. **43 of 76 saves still breach D14's 2 000 ms.** Risk 3's remaining
  escalation — *a save path that does not block the tick* — is unspent, it is the only lever that
  touches the stall rather than the cost, and it is **the owner's call, not a package's**. M5's
  exposure is Risk 3: the cadence ships as a package default onto machines nobody here can retune.
- **Slot 1 is the fragile world at ×100.** It oscillates 4–23 against its neighbours' 41–63, and
  at an achieved 25–32× it lives several times more simulated time per wall second than the worlds
  feeding it. Read it against `simulatedTime`, not wall clock. It is the world most likely to
  distort a WP5 churn measurement taken on the live map.
- **`genomeGaps` sits at ~117 000 (2026-08-11; was ~46 000–65 000 the day before) with no drain
  window.** The rate cap is the mechanism and
  the magnitude is not the finding — **the absence of a drain window is**. This is the number
  **Risk 7** must be read against, not the 2 714 the item was opened on: after M5 a departed
  stranger makes an entry permanent, and a monotone queue at this size is what §21's bounds exist
  to stop making each archive reconnect more expensive than the last.
- **The `8796` firewall rule and portproxy have no record of being run.** The owner steps exist in
  `dev_environment.md` and the reboot ritual verifies only `8795`, so the status page is
  effectively loopback-only from off this machine. **M5 makes the question moot rather than
  answering it** — WP3 hosts the archive publicly behind a DNS name and TLS — but the connection is
  worth carrying: the participant-facing view WP7 owes is the same page, and it has never actually
  been reachable by anyone but the owner.

## Session-resume checklist

1. **`git fetch` first.** Parallel sessions share this checkout. Reconcile against `origin/main`
   before staging anything, and stage explicit paths only.
2. **Read the workspace and attention state if AI-viz is up.** That is where a standing picture of
   in-flight work and anything awaiting the owner's answer lives.
3. **Check rig health before any operation**, off `/api/status` rather than off a log: 6/6 slots
   live and `modConnected`, 24/24 lanes `peer_live` with no bypass, `heldDepth` and
   `timeoutBounces` at 0 — `pacedDepth` and `custodyDepth` fluctuate on a healthy map (2–42 and
   22–58 measured at the bar on 2026-08-11; hard-fail only on the first two) — and the five
   local slots reporting `saveMinutes 10`, `saveKeep 6` and `timeScale 100`. A slot reporting `2`/`4` came up from a stale environment and needs its game
   restarted, not the rig. If the page does not answer, check whether the archive is inside its
   replay before concluding anything — minutes, growing with the ledger (~150 s on 2026-08-11).
4. **Check the memory directory is current** —
   `/home/ubuntu/.claude/projects/-mnt-wsl-data-bibites-multiverse/memory/MEMORY.md` and what it
   indexes — and update it if this document has moved past it.
5. **Then read the work-package table above**, pick the next startable package by the recommended
   sequence, and read that package in `m5_considerations.md` before dispatching an agent at it.
