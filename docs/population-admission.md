# Population-aware inbound admission

Status: implemented; production rollout is `adaptive-shadow` (non-enforcing).

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

## Admission modes

| Mode | Behavior |
|---|---|
| `off` | No population decision. The journal queue ceiling still applies. |
| `fixed` | Reject a new inbound migration when committed load reaches `inboundPopulationLimit`. |
| `adaptive-shadow` | Learn and publish an adaptive limit and closed/open decision, but never reject. This is the rollout default. |
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
control the result. The default target is ×10 and the learned result is clamped to 10–200. A
decrease moves by at most 10% per sample and an increase by 5%, so one changing interval does not
open or close the gate by dozens of organisms.

The last hour of valid samples and the learned limits are atomically persisted in
`<data-dir>/admission-state.json`. A restart therefore does not create a ten-minute fail-open
window, and a reviewed promotion from `adaptive-shadow` to `adaptive` uses the shadow evidence it
already collected. Changing the target, bounds, or safety margin deliberately starts learning
fresh.

The mod reports `targetTimeScale` separately from applied `timeScale`. A world deliberately set
below the configured target contributes no sample; without this distinction, an intentional ×5
world would look like a machine failing to hold ×10.

## Configuration

```text
--inbound-admission                    MULTIVERSE_INBOUND_ADMISSION
--inbound-population-limit             MULTIVERSE_INBOUND_POPULATION_LIMIT
--inbound-target-time-scale            MULTIVERSE_INBOUND_TARGET_TIME_SCALE
--inbound-population-min               MULTIVERSE_INBOUND_POPULATION_MIN
--inbound-population-max               MULTIVERSE_INBOUND_POPULATION_MAX
--inbound-population-hysteresis        MULTIVERSE_INBOUND_POPULATION_HYSTERESIS
```

For an immediate fixed rollout, use for example:

```text
MULTIVERSE_INBOUND_ADMISSION=fixed
MULTIVERSE_INBOUND_POPULATION_LIMIT=40
```

Do not enable `adaptive` in production until shadow history shows stable learned limits across
save cycles, restarts, and representative ecology.

## Observability and rollout gates

The peer stats block, `/api/status`, archived `metrics.jsonl`, the settings card, and local
`multiverse-sidecar --my-slot` view publish the mode, target, estimate, effective limit, committed
load, sample count, closed/enforcing state, and rejection total.

The enforcement promotion gate is:

- at least 24 hours of shadow samples per production world;
- no sustained oscillation of the effective limit;
- no estimate pinned to the configured minimum or maximum without an explained workload;
- no regression in migration loss, capacity sheds, journal discard, or live/mod-connected state;
- a reviewed fixed fallback limit for each host class.

## Verification record

- Unit tests cover robust estimation, shadow non-enforcement, fixed hysteresis, target mismatch,
  directional next-world walking, source skipping, and refusal-set skipping.
- The integration test `TestLivePopulationRefusalSpillsToNextUntriedWorld` creates three live
  worlds: the first sends east, the second refuses at its fixed population limit, and the third
  spawns the same migration exactly once. The source journal records slot 2 in `refusedSlots` and
  `peer_refused` as the reroute proof.
- On 2026-08-23, five Windows worlds and six hosted Linux worlds ran the candidate in
  `adaptive-shadow`. All eleven retained their slots with mod and relay connected, published the
  requested-speed and admission fields, kept enforcement false, and reported zero capacity sheds
  and zero discarded journal bytes. The cloud activation and its host verification completed.
