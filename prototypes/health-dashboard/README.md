# Production telemetry dashboard design

Status: implemented in the production `/health` page. The static mockup remains a design fixture.

## Goal

The dashboard gives one organized visual model of the production system. It shows relationships,
rates, capacity, latency, errors, and data movement.

The page does not lead with conclusions or an action queue. Short labels and values replace
explanatory paragraphs.

## Page structure

| Order | Layer | Main visualizations |
|---|---|---|
| 1 | System map | Hosts, services, traffic paths, data paths, and current flow rates |
| 2 | Headline signals | Requests, latency, errors, peers, archive input, write lag, CPU, and memory |
| 3 | Compute | Resource history, pressure, per-service memory, process state, and restart marks |
| 4 | Cloud and services | Public paths, service state, transfer use, cost pace, and certificate age |
| 5 | Application and traffic | Request volume, response codes, latency, relay sessions, worlds, and broadcast |
| 6 | Archive and data | Record pipeline, input and write rates, storage, roll-up, cold copy, and integrity |
| 7 | Data coverage | Source freshness, gaps, and collection cadence |

Each layer has a fixed color. Health state uses dots, borders, and missing segments. This rule
prevents one color from having two meanings.

## Visual rules

- All time charts use one time range and one aligned time axis.
- Deployment and restart marks appear on the charts.
- A gap means unknown data. A gap never means zero.
- When a rate gives more information, rate charts replace cumulative totals.
- Thresholds appear as lines or bands.
- Legends stay next to their charts.
- Units appear with each value.
- Every metric, chart, legend, and status mark has a hover and keyboard-focus tooltip.
- Tables are limited to endpoints, services, and other exact comparisons.
- Raw fields remain available below each layer, not on the overview.

## Data in the complete dashboard

### Available now

- Service-host CPU, load, memory, swap, disk, TCP, connection tracking, and service memory
- Scheduled monitor results
- World presence, population, migrations, routes, and achieved time scale
- Archive records, gaps, roll-up state, replay cost, raw segments, and cold-copy queue
- Broadcast viewer presence
- Transfer verdicts and billing freshness

### New collection required

- Request rate, response code, response size, and latency by public route
- Relay session count, duration, close code, and bytes by slot
- Service-host pressure-stall information
- Central world-host performance history
- External TLS, API, and video-playback probes
- Time-series provider transfer, burst capacity, cost, and spot price
- Deployment, restart, and recovery marks in the same time store
- Off-host retention for every chart

## Implementation

The production implementation is in `go/internal/archive/health_page.go`. It keeps the mockup's
organization and visual grammar, but it removes scenarios and static values. It reads
`/api/health`, `/api/status`, and `/api/viewers`. A missing collector becomes a gray data gap.

The implementation uses the current two-hour host window. The complete design still calls for a
shared off-host time store. That store is not part of this change.

## Mockup

Open [mockup.html](mockup.html). The file is standalone and uses static data. The scenario control
changes the visual state for typical load, compute pressure, and an archive stall.

The mockup defines information structure and visual grammar. It does not show live results or
define production thresholds and storage.
