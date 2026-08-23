# Production health dashboard end-state proposal

Status: design proposal with a static mockup. This document does not specify a production change.

The current dashboard proves that many measurements exist. It does not give the operator a fast
answer about service condition. Raw checks, host counters, archive counters, and coverage gaps all
have similar visual weight.

The end-state dashboard must answer these questions in order:

1. Is the service available now?
2. Is the permanent record current and safe?
3. What needs operator action?
4. What changed before the current condition?
5. Is performance stable, or is a resource approaching its limit?
6. Is the evidence fresh and complete?

## Proposed status model

The top banner reports service impact. It does not copy the worst monitor severity.

| State | Meaning |
|---|---|
| Outage | The public path is unavailable, or the archive does not record the map. |
| Degraded | The service runs, but participants, routing, record integrity, or performance are degraded. |
| At risk | The service runs normally, but a limit, stale audit, or maintenance gate needs action. |
| Healthy | All required service results are fresh and inside their limits. |
| Unknown | Required evidence is absent or stale. |

The underlying monitor severity remains visible in the evidence drawer. A critical maintenance
gate can appear as `At risk` when it has no current service impact. The dashboard must state that
distinction.

## First-screen hierarchy

### 1. Current condition

Show one state, one plain-language sentence, and the update time. Show four short facts beside it:

- Public path
- Permanent record
- Connected worlds
- Evidence confidence

### 2. Needs attention

Rank active items by impact and urgency. Each item contains four fields:

- Impact
- Evidence
- Next operator action
- Time in this state

Do not show one card for every monitor check. Put related symptoms into one operator problem. For
example, dark worlds and route bypasses are one participant-availability problem.

### 3. System domains

Group the system into four stable domains:

- Public access and broadcast
- Worlds and routing
- Archive and record integrity
- Host capacity and cost

Each domain shows one state, one sentence, and at most three supporting values.

### 4. Trends that change decisions

Show the current value, recent range, decision limit, and direction. The first set contains:

- Live worlds
- Memory available
- Swap used
- Disk used
- Replay time
- Genome-gap rate

Do not graph cumulative counters unless the dashboard converts them into a rate or delta.

### 5. Recent changes

Join incidents, recoveries, deployments, and monitor transitions on one timeline. This timeline
gives the operator a starting point for cause analysis.

## Progressive disclosure

Keep these items below expandable controls:

- All monitor results
- Per-unit memory
- TCP counters
- Per-world rows
- Raw archive counters
- Coverage inventory
- Data-source timestamps

The first screen can link to a focused detail page for each system domain. The main page must not
become the detail page.

## Evidence confidence

Freshness and coverage are properties of each result. They are not a separate wall of status cards.

Each domain shows one confidence label:

- Complete: all required sources are fresh.
- Partial: the conclusion uses fresh data, but a useful source is missing.
- Stale: a required source is late.
- Unknown: no current conclusion is possible.

Missing synthetic playback, world-host telemetry, or an external probe can reduce confidence. A
missing optional profiler does not make the live service unhealthy.

## Data required for the real dashboard

The first implementation can use the current APIs for service state, host samples, map state, and
viewer presence. It also needs a small presentation model that produces operator problems instead
of raw check cards.

The complete end state also needs these sources:

- An external public-path and video-playback probe
- Off-host history for service-host and world-host measurements
- World-host pressure-stall measurements in the central store
- Relay close codes and per-slot session counters
- Deployment and recovery events in a queryable feed
- Explicit `warming` state for rate checks without a complete time window
- A current billing reconciliation result

## Mockup

Open [`mockup.html`](mockup.html). The file is standalone and uses mock data. The scenario control
shows the proposed hierarchy during degradation, normal operation, and an outage.

The mockup is for information design. It does not define the final colors, type, thresholds, or
production API.
