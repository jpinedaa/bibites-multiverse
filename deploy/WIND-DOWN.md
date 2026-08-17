# Public wind-down policy

A hosted map is a bounded service.
The operator must publish its period before a participant joins.

This document defines the public commitments for the end of a run.
It does not contain commands for a live deployment.
The private operations runbook owns the exact resources, backups, and retirement commands.

## What ends

The shared relay, archive, website, and operator support end on the announced date.

A participant world does not end.
Its saves, journal, and local genome files remain on the participant machine.
The world continues as a normal Bibites world without the shared map.

The public source also remains available.
Another operator can create a separate map with a separate name and new credentials.

## Notice timeline

Use the service end date as the reference point.

| Time | Public commitment |
|---|---|
| Before the first join | Publish the period, retention rule, and wind-down policy. |
| 30 days before | Remind participants of the date and record disposition. |
| 14 days before | Explain what remains on each participant machine. |
| 7 days before | Send the final reminder or announce an extension. |
| End date | Stop new map activity and preserve the final service record. |
| Within 7 days | Complete the announced record disposition and publish a closing notice. |
| After the close | Retire billable resources under the private operations runbook. |

The operator can extend the period with an announcement.
The operator must not shorten it silently.

If an early close is necessary, give a dated notice as soon as possible.
Use the same record and participant protections as a planned close.

## Service stop requirements

The private runbook must implement this order:

1. Capture the final public status and summary data.
2. Stop new relay activity.
3. Let the archive finish its final writes.
4. Stop the archive and background monitors.
5. Create and check the final backup.
6. Publish a static closing page or notice.
7. Apply the selected retention rule.
8. Retire cloud and DNS resources only after recovery checks pass.

Do not copy these steps into a public command sequence with live resource names.
The operator must resolve the exact targets from private inventory before a retirement action.

## Durable records

Each deployment must select and publish one retention rule.
The completed parameter file records the selection in `MV_RETENTION`.

The public kit supports these policy shapes:

| Rule | Record of what crossed | Raw crossing lines | Genome blobs |
|---|---|---|---|
| `keep-everything` | Keep it all. | Keep every line on the server. | Keep all stored blobs. |
| `bounded-ledger` | Keep only the announced post-run horizon. | Keep only that horizon. | Apply the announced blob rule. |
| `prune-genomes` | Keep it all. | Keep every line on the server. | Remove blobs outside the announced horizon during the run. |
| `graduate` | Export the approved catalog seed. | Apply the announced catalog policy. | Apply the announced catalog policy. |
| `rollup-window` | Keep it all, for the run and beyond. | Keep them on the server for the announced window, and off the server for the run. | Remove blobs outside the announced horizon during the run. |

`rollup-window` is three tiers and not two, so it needs its own explanation.

The record of what crossed is every count, date, and family link.
It is kept for the whole run and beyond. Nothing removes a species, a count, or
a family link.

The raw crossing lines are the individual entries behind those counts.
The server keeps them for the announced window, which is the same period as the
genome horizon. A line older than the window is on the off-server copy only.

A line does not leave the server until a confirmed copy exists off the server.
The operator cannot make it leave sooner.
This is a capacity rule, not a deletion service. It cannot name a peer, a
species, or an organism, so it is not a takedown and must not be offered as one.

The deployment must explain the exact result before participants join.
Do not change to a shorter horizon without a new participant notice.
Announce a change from `keep-everything` to `rollup-window` before you apply it.

### Credential verifiers

The credential store contains verifiers, not participant secret values.
It has no authentication purpose after the relay is retired.

The private runbook must define its deletion and recovery boundary.
Do not publish the store or include it in a public final snapshot.

### Slot register

The slot register makes ledger records understandable.
Keep it with any retained ledger unless the published policy requires full deletion.

### Final summary

Publish a sanitized final summary when practical.
It can include aggregate map, species, flow, and service totals.

Do not publish these items:

- Join strings or credential material.
- Raw participant saves or genomes.
- Raw service logs.
- Private addresses or cloud resource identifiers.
- Alert, support, or billing records.

## Backups and recovery

Create the final backup before you remove any resource.
Check its manifest and checksums.
Record its storage location, retention date, and owner in private operations storage.

A provider snapshot alone is not a complete retention policy.
The operator must know which files the snapshot contains and when it expires.

Do not claim that a deleted record is recoverable unless a checked copy exists.

## Closing notice

The closing notice must state these facts:

- The announced run is complete.
- Participant worlds remain on participant machines.
- The selected record disposition is complete or has a dated completion plan.
- A sanitized final summary is available, if one was created.
- The public source remains available for a new, independent map.

Keep the exact notice and delivery record in private operations storage.
