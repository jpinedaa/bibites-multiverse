# M5 public-release record

> [!NOTE]
> This record was frozen after the `0.1.0` publication on 2026-08-15 UTC.
> Read [`STATUS.md`](STATUS.md) for the public project state.

M5 changed Bibites Multiverse from a private development grid into a public experiment.
This document records its decisions, delivered work, and test evidence.

This document is not a live tracker or an operator runbook. It does not contain current peer
state, service health, join-string activity, resource inventory, costs, incidents, or rollout
receipts.

## Scope and authority

| Question | Public source |
|---|---|
| What is the current public phase? | [`STATUS.md`](STATUS.md) |
| Why did M5 choose this design? | [`m5_considerations.md`](m5_considerations.md) |
| What project decisions bind later work? | [`system_decomposition.md`](system_decomposition.md) |
| What does each protocol require? | [`contracts/contract-a.md`](contracts/contract-a.md) and [`contracts/contract-b-m4.md`](contracts/contract-b-m4.md) |
| What did M5 deliver? | The work-package record below |

The design document remains the source for arguments, risks, and package definitions.
This record reports only the result and its public evidence.

## Decided state

The owner ratified nine decisions on 2026-08-10. The owner refined two compatibility decisions
on 2026-08-11. Five decisions became D21 through D25 in the system design.

| Decision | Result | Design source |
|---|---|---|
| D21 — public wire | Per-peer credentials and TLS shipped together in `contract-b/4`. Contract A gained its bearer token in `contract-a/2.4`. | M5 decision 1 |
| D22 — compatibility | Contract version is the network compatibility test. Game version remains diagnostic metadata for new readers. | M5 decision 5 |
| D22 — retained gates | The four existing game-version gates stayed in place. The design defers cross-version loading until version skew causes a real problem. | M5 DQ5 |
| D23 — control surface | M5 kept the public operator surface read-only. Write controls moved to M6. | M5 decision 2 |
| D24 — service period | The first announced service period runs from August 14 through November 14, 2026. Any extension needs a public announcement. | M5 decision 4 |
| D25 — release channel | GitHub Releases carries packages, checksums, security guidance, and the default configuration. | M5 decisions 6 and 7 |
| Archive retention | The migration ledger stays permanent. Genome blobs use a configurable horizon with a 30-day hosted default. | M5 decision 3 |
| Exit bar | The test needs four non-owner peers, 72 continuous hours, and no operator action on participant computers. | M5 decision 8 |
| Velocity floor | M5 did not add a velocity floor. The public experiment measures whether one is necessary. | M5 decision 9 |

The exact protocol changes are in Contract A section 21 and Contract B sections 22 and 23.

## Work packages

The package names and completion bars come from `m5_considerations.md`, *Work Packages*.
The states below are the frozen delivery state.

| WP | Delivered result | Frozen state |
|---|---|---|
| WP1 — contract amendments | Published `contract-a/2.4`, `contract-b/4.0`, and the M5 compatibility rules. Reverified `bb8-genome/1`. | Complete, 2026-08-11 |
| WP2 — transport security | Added TLS, peer-bound credentials, archive subscriber authorization, and the Contract A bearer token. | Complete, 2026-08-11 |
| WP3 — hosted publication | Published the website, relay, archive, service safeguards, announced period, and B26 forward receipt. | Complete, 2026-08-15 |
| WP4 — capacity and abuse controls | Added configurable frame limits, the authenticated admin path, renderer escaping, and participant-visible limit data. | Complete, 2026-08-12 |
| WP5 — placement under churn | Added holes-before-growth placement, quiet reclaim, bounded status coalescing, and a synthetic churn harness. | Complete, 2026-08-11 |
| WP6 — participant packages | Published Windows and Linux add-on archives, checksums, support data, installers, and safe uninstallers. | Complete, 2026-08-15 |
| WP7 — support surface | Added participant guides, error taxonomy, `--diagnose`, `--my-slot`, and renderer-escaping tests. | Complete, 2026-08-15 |
| WP8 — public playtest | Measures the exit bar, traffic, version skew, archive growth, and velocity-floor evidence. | Open at freeze |

At the freeze, WP8 was the only open package. A completed playtest belongs in
`m5_findings.md`, with a link from `STATUS.md`.

## Stable delivery evidence

### WP1 — contracts before code

M5 wrote the public protocol before it changed implementations. The final public identifiers are:

- `contract-a/2.4` on the local mod-to-sidecar connection.
- `contract-b/4.0` on `/contract-b/v4`.
- `bb8-genome/1` for canonical genome identity.

The contract update retained the existing game-version gates as named exceptions.
It also marked game-version fields as diagnostic data for new readers.

### WP2 — the coordinated protocol crossing

The local development grid crossed both protocol boundaries in one 7-minute 18-second window.
The test reported zero discarded journal bytes and no archive ledger gap.

The sequence proved four reusable rules:

1. Mint peer credentials before the old network path closes.
2. Start the new sidecars before game plugins that require Contract A tokens.
3. Stop incompatible network versions from exchanging traffic.
4. Make sure that journal replay succeeds. Then accept the rollout.

The Contract A token changed the plugin DLL. A running game locks that DLL.
This fact made the protocol crossing one coordinated game-stop window.

### WP3 — hosted publication

The public service appeared at
[`bibitesmultiverse.com`](https://bibitesmultiverse.com/) before participant enrollment.
The publication included these tests:

- HTTPS and certificate renewal.
- Process recovery after exit and host restart.
- Monitoring delivery to a person.
- Identity backup and restore with matching hashes.
- Public period and wind-down text.
- Website security headers and service probes.

The B26 forward receipt shipped with the service code.
Its cost at public traffic rates remained WP8 evidence at the freeze.

This record keeps the public outcome. It omits provider inventory, account identifiers,
addresses, costs, current alarms, and operator receipts.

### WP4 — limits and support controls

Every admission limit is configurable and countable from protocol frames.
The relay publishes peer-visible limits without inspecting opaque organism data.

The admin interface uses a separate authenticated listener. Its destructive calls require a
confirmation value tied to current ring state. Renderer escaping has an automated test.

### WP5 — churn harness

The synthetic harness processed 50 million events in 5 minutes 42 seconds.
Seven placement invariants stayed clean.

The run showed these results:

- Returning peers did not spend new addresses.
- The relay did not issue the same address twice.
- Holes filled before an axis grew.
- Status broadcasts stayed below the test bound.

The rectangle does not shrink. Long-lived maps therefore retain address history.

### WP6 — packages

The release builder creates add-on archives for Windows and Linux.
Authorized game payloads can also produce complete editions outside the public release.

The Windows install and uninstall suite passed 65 tests. The Linux suite passed 109 tests.
Release `v0.1.0` published both add-on archives and `SHA256SUMS`.
Fresh downloads passed checksum and ZIP-integrity tests.

### WP7 — participant support

The support surface names a remedy and responsible actor for each refusal.
It includes installation, joining, diagnosis, leaving, support-matrix, and defaults guidance.

The sidecar exposes two local support commands:

- `--diagnose` examines the participant-visible setup and returns stable exit codes.
- `--my-slot` shows one participant only their own world state.

## Historical delivery order

M5 used this dependency order:

1. WP1 defined the protocols before implementation.
2. WP2 and the WP7 documentation structure advanced together.
3. WP3 and WP4 advanced together. WP5 followed against the settled relay.
4. WP6 followed stable defaults and a stable support matrix.
5. WP7 closed before the public playtest.
6. WP8 followed the local churn and abuse rehearsals.

This order remains useful for a future protocol major. It does not define a live rollout
procedure.

## Open public evidence at freeze

At the freeze, the public experiment still needed to report these results:

- The full M5 exit bar.
- Crossing and epoch rates with non-owner peers.
- Per-peer archive growth.
- Forward-receipt cost at public traffic rates.
- Broadcast volume under real churn.
- Version-skew partition behavior.
- Evidence for or against a migration velocity floor.
- The opaque cross-version payload assumption, clearly marked as untested.

The experiment can change the findings. It does not change the frozen delivery record.
