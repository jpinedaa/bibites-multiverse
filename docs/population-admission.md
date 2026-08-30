# Population-aware inbound admission

Status: implemented. Operator-controlled production worlds enforce `adaptive`; participant
packages retain the `adaptive-shadow` default.

## Why this exists

The blue portals were not blocked at capture. A receiving sidecar could refuse a new migration
with `OVERLOADED` before taking custody, but the sender treated the destination's continued
liveness as a reason to offer the organism straight back to that same destination. A full live
world could therefore absorb retries without visibly exporting many organisms.

Queue depth and living population are different loads. The existing `inboundQueueMax` of 64
protects the sidecar journal; it does not stop a world whose game population is already too large
for its CPU budget from receiving more organisms.

## Evidence and sizing

The production history used for this change contains 11,401 stable five-minute cloud samples.
Living population and achieved time scale have correlation `-0.738`: more organisms strongly
coincide with a slower world on the shared host. Across hosted slots, the median empirical machine
budget (`population × achievedTimeScale`) is 412–462. At a desired ×10 that implies a median cap of
41–46 organisms; the lower-tail estimate is 24–34. These are observations, not a universal static
limit, which is why the adaptive controller learns each machine separately.

## Routing rule

An `OVERLOADED` receiver NACK is emitted before a journal write, so it proves the receiver took no
custody. The source now:

1. records that destination in the migration's durable `refusedSlots` set;
2. continues in the original direction on the original row or column;
3. skips the source, dark/incompatible worlds, and every destination already tried;
4. preserves the migration ID, payload, edge, position, velocity, heading, and lineage;
5. remains bounded by `maxReroutes` and `bounceTimeoutMs`.

When every alternate is unavailable, the source honors the receiver's 15-second retry hint. It
may retry the current live world because population pressure is transient, but it does not reset
the original bounce deadline.

A relay transport refusal is a different event. A queue-full `NOT_FORWARDED` means the
destination transport queue did not accept that exact attempt. The source uses its bounded
progress path only when all these conditions are true:

- `neverForwarded` is present for mixed-version discrimination;
- `relaySessionId` matches the current committed attempt;
- `refusedAttempt.destSlot` and `refusedAttempt.rerouteCount` match the current journal state;
- no `FORWARD_RECEIPT` exists for that session and destination.

The legacy `neverForwarded` value can be false after an earlier accepted attempt. The exact
`refusedAttempt` still proves that the later queue rejected this attempt. A graceful-drain
`NOT_FORWARDED` omits all three proof fields. It does not consume a destination or start the
refusal deadline.

The source records `refused`, the transport-refused slot, and the first deadline in one durable
update. That update clears the old attempt's relay session and `sentAt`. It then tries the next
compatible, untried world on the original axis. The migration ID, body, edge, geometry, velocity,
heading, and lineage do not change. A present reroute count is positive and increments on every
rewrite. The tried set prevents a destination from repeating. The increasing count makes each
destination and count pair unique in the chain.

The first deadline is `bounceTimeoutMs` after the first exact refusal. A reroute, retry, reconnect,
restart, or journal compaction does not reset it. Exhausting the walk or `maxReroutes` also ends
the chain. Each terminal condition bounces the migration once.

Missing, stale, mismatched, or contradicted attempt proof leaves the durable state and scheduler
unchanged. A delayed peer NACK from an earlier destination has the same result. Before a pending
alternate sends, the sidecar refreshes current journal state under its scheduler lock. It checks
the first-refusal deadline again after encoding and pace admission. It then fsyncs fresh
current-attempt session and `sentAt` values before socket enqueue. A crash or enqueue error cannot
cause another send. A committed entry can receive an answer or become lost. It cannot bounce on
its earlier refusal deadline.

This behavior needs a `contract-b/4.2` relay. A 4.2 sidecar with a 4.1 relay safely treats missing
attempt correlation as no proof. Deploy the relay first, then sidecars. Graceful drain remains a
wait-and-reconnect condition during the rollout.

## Admission modes

| Mode | Behavior |
|---|---|
| `off` | No population decision. The journal queue ceiling still applies. |
| `fixed` | Reject a new inbound migration when committed load reaches `inboundPopulationLimit`. |
| `adaptive-shadow` | Learn and publish an adaptive limit and closed/open decision, but never reject. This is the participant-package default. |
| `adaptive` | Enforce the learned limit after the estimator is ready. Before readiness it fails open. |

Committed load is the last heartbeat population plus inbound journal entries plus successful
spawns not yet reconciled by the next heartbeat. This prevents the heartbeat/ACK race from
admitting a burst against a stale population reading. A closed gate reopens only at
`limit - hysteresis`.

## Adaptive estimate

At most once per wall-clock minute, after the sidecar has a continuous achieved-speed window, it
records:

```text
machine budget = living population × achieved time scale
raw limit       = floor(median(last hour of budgets) × 0.90 / target time scale)
```

Ten valid samples are required. Save stalls are outliers and the median makes them unable to
control the result. The ordinary policy starts at ×10 and then follows the game's current requested
speed. An operator can instead set an explicit fixed target. The learned result is clamped to
10–200. A decrease moves by at most 10% per sample and an increase by 5%, so one changing interval
does not open or close the gate by dozens of organisms.

The last hour of valid samples and the learned limits are atomically persisted in
`<data-dir>/admission-state.json`. A restart therefore does not create a ten-minute fail-open
window, and a reviewed promotion from `adaptive-shadow` to `adaptive` uses the shadow evidence it
already collected. Changing an explicit fixed target, the bounds, or the safety margin deliberately
starts learning fresh. In the ordinary requested-speed policy, a slider change keeps the last
hour's machine-budget samples and immediately recalculates their limits for the new target.

The mod reports `targetTimeScale` separately from applied `timeScale`. The ordinary controller
adopts each positive, finite requested target, even while the world is paused or before an achieved
speed window is ready. Those heartbeats can change the target but cannot add a capacity sample.
With an explicit fixed target, a world requested below that target contributes no sample; without
this distinction, an intentional ×5 world could look like a machine failing to hold ×10.

The local broadcast runner explicitly fixes both the game and admission targets at ×6.5.
The participant package starts the game at ×10 but does not set a fixed admission target. If a
participant selects ×5, the controller learns and publishes a ×5 population limit; returning to
×10 is not required.

## Configuration

```text
--inbound-admission                    MULTIVERSE_INBOUND_ADMISSION
--inbound-population-limit             MULTIVERSE_INBOUND_POPULATION_LIMIT
--inbound-target-time-scale            MULTIVERSE_INBOUND_TARGET_TIME_SCALE (optional fixed target)
--inbound-population-min               MULTIVERSE_INBOUND_POPULATION_MIN
--inbound-population-max               MULTIVERSE_INBOUND_POPULATION_MAX
--inbound-population-hysteresis        MULTIVERSE_INBOUND_POPULATION_HYSTERESIS
```

For an immediate fixed rollout, use for example:

```text
MULTIVERSE_INBOUND_ADMISSION=fixed
MULTIVERSE_INBOUND_POPULATION_LIMIT=40
```

The normal promotion path is to enable `adaptive` only after shadow history shows stable learned
limits across save cycles, restarts, and representative ecology. An operator may authorize an
earlier controlled-world promotion to collect enforcement evidence, but the exception, initial
health evidence, fixed fallbacks, and rollback conditions must be recorded below.

## Observability and rollout gates

The peer stats block, `/api/status`, archived `metrics.jsonl`, the settings card, and local
`multiverse-sidecar --my-slot` view publish the mode, target, estimate, effective limit, committed
load, sample count, closed/enforcing state, and rejection total.

The map shows `OPEN · WAITING ×n` when an explicitly fixed admission target is above the
requested speed. Under the ordinary policy, the displayed admission target follows the requested
speed, so selecting ×5 does not produce a ×10 waiting state. It shows
`OPEN · TARGET UNKNOWN` when the world does not report its requested speed. These states identify
why the sample count does not increase.

The live map deliberately distinguishes an **offer** from a **delivery**. A lane's numeric rate and
recorded migration count come from copied `MIGRATION_PAYLOAD` offers, so they can continue rising
beside a closed admission gate. Every world cell separately publishes the fresh effective
population limit and one of `OPEN`, `CLOSED`, `OPEN · LEARNING`, shadow, off, or unknown. The cell
keeps its live state while the admission outline turns closed: network liveness and permission for
a new inbound organism are different facts.

The moving creature and destination flash come from `/api/hops`, which correlates an offer with the
receiving peer's `MIGRATION_ACK`; that ACK is sent only after the destination game acknowledges the
spawn. The bounded correlator now also carries the ordered slots that explicitly NACKed that same
migration before its final ACK. The page replays a confirmed spillover as two evidence-backed
events: the glyph reaches the refused world's boundary and shows `REFUSED → SLOT n`, then the same
glyph restarts at its source and follows a temporary dashed bypass to the world that actually
acknowledged it. It never flashes the refused cell as an arrival.

A standalone admission NACK still produces no moving creature. Rejected attempts are replayed only
when that same migration later has a confirmed delivery, so the animation cannot turn routing
pressure into a population claim. If `/api/hops` is unavailable, the page pauses delivery animation
instead of guessing an arrival from the offer counters. The refusal chain is ephemeral and bounded
with the hop correlation; it is not added to `metrics.jsonl` or to the permanent migration ledger.

`CLOSED` applies to **new offers**. It does not revoke custody the world already accepted before
the gate closed, because discarding those journal entries would lose organisms after handoff. Read
`pacedDepth` beside the gate: while it is above zero, those previously accepted arrivals can still
reach the game and produce genuine ACK-confirmed animations. Once it drains to zero, a closed gate
has no accepted backlog left to deliver.

The enforcement promotion gate is:

- at least 24 hours of shadow samples per production world;
- no sustained oscillation of the effective limit;
- no estimate pinned to the configured minimum or maximum without an explained workload;
- no regression in migration loss, capacity sheds, journal discard, or live/mod-connected state;
- a reviewed fixed fallback limit for each host class.

The service already archives these fields in `metrics.jsonl`, so an enforcing rollout does not
need a second telemetry path. Evaluate deltas rather than isolated cumulative counters. At 24-hour
and 72-hour checkpoints, compare achieved speed and population against effective-limit changes,
then review rejection, spillover, lost-forward, custody-depth, capacity-shed, journal-discard, and
live/mod-connected deltas. A rejection is expected evidence that the gate worked; a custody loss,
discard, or sustained loss increase is not.

## Controlled enforcement record

On 2026-08-23 the operator explicitly authorized early enforcement on the eleven worlds under
direct control so real refusal and spillover evidence could be collected. Five Windows worlds
and six hosted Linux worlds retained their learned `admission-state.json` and switched from
`adaptive-shadow` to `adaptive`; the public participant default did not change. The initial
post-promotion snapshot reported all eleven live, mod-connected, and enforcing, six closed by
their learned limits, continuing migrations, zero capacity sheds, and zero discarded journal
bytes. The cloud rollout used the guarded runtime transaction and passed host verification.

The first 20-minute watch recorded a bounded but important follow-up. Hosted slot 3 wrote 22
forwards off after the five-minute answer deadline: 15 had relay receipts naming newly closed slot
6 during one post-activation burst, while the later seven named participant slot 19 around a mod
disconnect. Pre-promotion logs also contain clustered shadow-mode timeouts from hosted slots 2 and
5 to overloaded local slot 9, so the post-promotion count alone is not evidence that admission
caused a regression. Preserve both baselines and investigate the missing ACK/NACK path. Do not
infer that a receipt proves destination custody, and do not loosen at-most-once routing to hide the
counter.

A later production-shaped discriminator isolated a separate source-side wedge. Twelve public
samples used 30-second intervals. Slot 7 reported positive rates on all four lanes in samples
1–6, then zero rates on all four lanes in samples 7–12. It stayed live, mod-connected, populated,
and open on four edges while `custodyDepth` stayed at 64.

During the zero period, its read-only journal diagnostic reported zero pending ACKs, zero paced
arrivals, and zero unresolved sends. It also reported no discarded bytes. This combination
identifies refused or pending outbound entries at the cap. It does not identify a sent timeout or
an ACK drain. Contract B §31 defines the attempt-scoped correction without weakening the receipt
rule.

The immediate rollback is `adaptive-shadow`, which preserves estimator and journal state. If an
enforcing fixed fallback is needed while the adaptive calculation is investigated, the reviewed
starting points are 40 organisms for each hosted world and 10 for each world on the shared local
Windows machine. Roll back if any world loses liveness or mod connectivity, capacity sheds or
journal discards appear, lost-forward deltas rise persistently, or limit oscillation produces
unacceptable routing pressure. Do not delete `admission-state.json` or migration journals during
rollback.

## Verification record

- Unit tests cover robust estimation, shadow non-enforcement, fixed hysteresis, target mismatch,
  directional next-world walking, source skipping, and refusal-set skipping.
- On 2026-08-30, requested-target regressions proved that an ordinary ×5 participant heartbeat
  learns and publishes a ×5 limit, a paused slider change immediately reapplies retained budget
  samples without adding one, valid flag and environment overrides remain fixed, an invalid
  environment value remains omitted, and persisted ×5 state restores only under the following
  policy. The full Go vet/test suite, Windows and Linux sidecar cross-builds, tracked Markdown link
  check, release-link test, runtime-rule test, and release-version consistency check passed.
- The integration test `TestLivePopulationRefusalSpillsToNextUntriedWorld` creates three live
  worlds: the first sends east, the second refuses at its fixed population limit, and the third
  spawns the same migration exactly once. The source journal records slot 2 in `refusedSlots` and
  `peer_refused` as the reroute proof.
- Archive regressions stage an offer to a closed peer, record its NACK, stage the same migration ID
  to the next peer, and require exactly one live-map hop at the peer that ACKs. A second regression
  observes that ACK before the rerouted payload copy and requires peer matching to keep it off the
  rejected destination.
- The confirmed-hop feed retains multiple receiver refusals in attempt order and ignores a delayed
  NACK from an earlier receiver after a later attempt is current. Page regressions require every
  map cell to repaint its effective limit and gate state, require the refusal marker and derived
  final route, and prohibit attaching the confirmed destination to the original lane geometry.
- `TestSixtyFourTransportRefusalsUseTheAlternateThenBounceOnce` starts with 64 outbound entries.
  It proves two same-axis queues refused each entry, then verifies one bounce per migration. A
  valid local acknowledgment clears total custody below the 64-entry cap.
- `TestPre42RefusedReplayEntersAttemptScopedBoundedWalk` replays 64 pre-4.2 `refused` entries.
  Each entry retains its old relay session and send clock but has no refusal deadline. After the
  clock and session change, each old global proof permits one safe retry. The retry replaces the
  stale attempt metadata and cannot enqueue twice. An exact 4.2 refusal then moves every entry to
  a distinct pending alternate with a durable deadline.
- `TestTransportRefusalProofSafetyMatrix` covers exact, missing, stale, mismatched,
  receipt-contradicted, proof-free drain, legacy 4.1, and wrong-code input. Only exact current
  attempt proof starts the bounded walk.
- Mixed peer-to-transport and transport-to-peer-to-transport tests cover a migration-wide false
  value with exact attempt proof. Delayed peer and relay NACK tests protect the pending alternate.
- Restart, compaction, session rotation, deadline, `maxReroutes`, crash, and closed-writer tests
  verify durable progress. They also verify that a committed alternate cannot retry or bounce.
- On 2026-08-23, five Windows worlds and six hosted Linux worlds ran the candidate in
  `adaptive-shadow`. All eleven retained their slots with mod and relay connected, published the
  requested-speed and admission fields, kept enforcement false, and reported zero capacity sheds
  and zero discarded journal bytes. The cloud activation and its host verification completed.
- Later on 2026-08-23, the operator-authorized controlled rollout promoted those same eleven
  worlds to `adaptive` without discarding their learned state. All eleven reported enforcement
  active after restart; the initial snapshot had six gates closed, migrations continuing, and no
  capacity shed or journal discard. `metrics.jsonl` now supplies the 24-hour and 72-hour evidence
  described above.
- The delivery-confirmed archive correction merged as `3617390` and passed hosted checks run
  `32681067865`. Its guarded production restart replayed in 97 seconds, held the participant gate
  for 104 seconds, and proved the archive subscription preceded every placement claim. Sixty
  seconds later the original 15 live and four dark slots were present. In a post-deploy 15-second
  interval, slot 9 rejected 27 new offers while `/api/hops` reported 148 confirmed deliveries to
  other slots and zero to slot 9. Its `pacedDepth` remained 12, explicitly preserving the older
  accepted backlog rather than treating `CLOSED` as permission to discard custody.
- The explicit-refusal live-map correction merged as `4ade74e` and passed hosted checks run
  `32729843166`. A provider snapshot and durable-file backup preceded its guarded production
  restart. Replay took 86 seconds, the participant gate was raised for 88 seconds, archive
  subscription preceded reopening, and the 60-second closeout had 14 live and five dark slots.
  Installed and running hashes matched, and all 34 host checks passed. The first sampled hop feed
  contained 60 confirmed reroutes with explicit refusal chains, including slot 5 reaching slot 4
  after slots 6 and 14 refused it. Slot 9 simultaneously published `OPEN · LEARNING` evidence:
  adaptive mode, zero samples, no effective limit, estimate 10, and enforcement false.
