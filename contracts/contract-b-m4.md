# Contract B — M4 (Sidecar ↔ Relay ↔ Sidecar ↔ Archive)

**Version:** `contract-b/3.0`
**Status:** implementation-ready for M4. Written 2026-08-05 from the ratified decisions
D12–D16 (`system_decomposition.md`), the amended D2, and the work order in
`m4_considerations.md`, *Contract Changes Needed*.
**Supersedes:** `contracts/contract-b-m3.md`, in full. That document is the historical
record of the M3 ring and is **not** current guidance.
**Companion documents:** `contracts/contract-a.md` (`contract-a/2.0`, mod ↔ sidecar) and
`contracts/genome-hash.md` (`bb8-genome/1`, the canonical genome projection — **unchanged by
M4**).

> ### Why a successor document and not an amendment
>
> `contract-b-m3.md` opened with this box for the same reason, and its three tests give the
> same answer again:
>
> 1. **The M3 file is milestone-named and milestone-scoped.** It is titled *M4*'s
>    predecessor — *Contract B — M3* — and its §1 opens "In scope for M3: one ring of three
>    slots across two physical machines". A file called `contract-b-m3.md` that describes a
>    two-axis map of six instances would be misleading in its filename, its title and its
>    first paragraph.
> 2. **The change is structural, not clarifying.** The ring becomes a grid; a reservation
>    gains a coordinate; `SECTOR_GRANT` returns a map of effective neighbours instead of one
>    east neighbour; `PEER_STATUS` gains positions, populations and a map shape; insertion is
>    rewritten for two axes; a lane re-pairs instead of closing; the relay gains an explicit
>    non-delivery answer and the record behind it; and the custody rules gain a handoff state,
>    a re-route and a bounded hold. Half the catalogue's semantics move.
> 3. **M3's text is still needed as it stands.** `m3_considerations.md`, the M3 exit-test
>    record and the T0/T1 baselines all cite it as the specification the passing rig ran
>    against. Overwriting it would destroy that record.
>
> **Contract A took the opposite decision for this same milestone**, and `contract-a.md` §15
> states why: its file is not milestone-named, its message catalogue does not change, and its
> body stays current with four marked edits. A major version bump is not by itself a reason
> to split a file — the version identifies the wire, the file identifies the interface.
>
> The rule for a reader is therefore simple: **this document is Contract B. The M3 file is
> history.** Nothing here inherits from it silently — where a rule carried over unchanged, it
> is restated here in full, and §11 lists everything that changed and why.

This document is written so a Go implementer building the relay, a Go implementer building
the sidecar, and a Go implementer building the archive can each build their side without
talking to each other. It reuses `contract-a.md`'s envelope, version rules and RFC 2119 key
words. Where Contract A already answers a question, this document points at it instead of
restating it.

---

## 1. Scope

In scope for M4: one **grid** — a 3×2 map of six slots across two physical machines on a LAN,
growing to a seventh in the exit test — one relay, one archive; coordinate addressing beside
the slot number; per-axis route-around past a dark slot or a hole; insertion between two live
slots on either axis; slot handover; an explicit relay answer that proves non-delivery; the
bounded hold and its automatic bounce; population and operational stats on the map view (the
"ring view" of D15); and the shared token on the wire, carried unchanged from M3 (D9).

Nothing here caps the map at six. Six is what the rig runs (`m4_considerations.md`,
Question 7); the rules are written for any rectangle, and §13 item 3 records what has not
been tested at a larger one.

Out of scope, and named so nobody builds them by accident: TLS and per-peer authentication
(M5), capacity and abuse limits (M5), `TOPOLOGY_GOSSIP` and `PEER_EXCHANGE` (M6),
`CATALOG_QUERY` and `CATALOG_RESPONSE` (M7), and any write interface on the archive — M4
records and reads only (D11). **Every milestone number in that list is D16's**: public
release is M5, direct P2P is M6, ecosystem completeness is M7.

---

## 2. Topology — the grid

M4 runs one **rectangular map** of slots. The M3 ring is the one-row case of it, and every
M3 rule survives as the height-1 specialization (D13).

```
   row 1:  ┌──► (0,1) ──► (1,1) ──► (2,1) ──┐
           └───────── east wrap ────────────┘
              ▲          ▲          ▲          north lanes wrap back to row 0
              │          │          │
   row 0:  ┌──► (0,0) ──► (1,0) ──► (2,0) ──┐
           └───────── east wrap ────────────┘

   The M4 target rig: a 3×2 map, six instances, two machines.
   Row 0 holds slots 1–3, row 1 holds slots 4–6.
   Each peer exports east and north. Each peer receives west and south.
   No lane returns to its own source.
```

| Term | Definition |
|---|---|
| **slot** | The **routing address** of a peer: an integer `≥ 1`. Never reused, never renumbered. `destSlot` names it. It replaces M2's sector and is unchanged from M3. |
| **position** | The **coordinate** `{col, row}`, both `≥ 0`. It decides who a peer's neighbours are, and nothing else. A position moves when the map grows; an address never moves. |
| **map** | The rectangle `{width, height}`: every `col` in `[0, width)` and every `row` in `[0, height)` is a position that exists. |
| **hole** | A position inside the rectangle with no reservation. A hole and a dark slot look identical to a router, which is why route-around is what makes a map with holes viable (D12). |
| **east lane** | From a position to the next **deliverable** slot east along its **row**, wrapping at the row's end. |
| **north lane** | From a position to the next **deliverable** slot north along its **column**, wrapping at the column's top. |
| **effective neighbour** | The slot a lane actually points at, after skipping holes and undeliverable slots (§8). Published in `SECTOR_GRANT`. |
| **structural order** | The registry in row-major order — `row` ascending, then `col` ascending. Published in `PEER_STATUS`. It is the shape; the effective lanes are the effect. |
| **slot reservation** | The binding of a slot **and its position** to a `peerId`. It survives disconnection, restart and reinstall, and **it does not expire** (D8). Only an operator releases or hands it over (§7.5). |

Five properties follow, and every rule in this document depends on them:

- **A slot belongs to a peer identity, not to a connection.** An offline peer keeps its slot
  and its position; its lane is routed around, and its reservation waits for it.
- **A slot number is never reused.** `maxSlotEverIssued` never decreases (§7.2). This is what
  makes a vacant slot unambiguous: a journaled `destSlot` that no longer exists names a world
  that never returns, so `SLOT_VACANT` is a **permanent** answer (§6.8).
- **Addresses never move; positions may.** A splice that inserts a column shifts the columns
  after it, so peers get new coordinates and new neighbours. No slot number changes, no
  journal entry is invalidated, and no organism in flight is affected (§7.3).
- **Out and in are different doors, per axis.** An organism that leaves through the east edge
  cannot come back through it; it must travel the whole row. The same holds for the column.
  There is no boomerang at a shared edge and no ping-pong to time out.
- **A dark slot no longer stops the current.** Its lane re-pairs to the next deliverable slot
  and the flow continues (D12). This is the property M3 lacked, and T1 measured what its
  absence costs: two hours of one dark slot raised square crossings about nine times at its
  west neighbour, and collapsed the inbound stream of every peer behind it.

### 2.1 Degenerate shapes

A lane needs a *third* party to route around a gap, and an axis of two has none. The
arithmetic is the same one that made a two-slot ring degenerate in M3, and it now applies per
axis:

| Shape | East lanes | North lanes | Note |
|---|---|---|---|
| `1×1` | none | none | One peer. Both export edges close with `no_peer`; a peer never exports to itself. |
| `w×1` (the ring) | a cycle of `w` | **none** | Every M3 rule, unchanged. The north edge is declared by the mod and stays closed with `no_peer` for the life of the map. |
| `1×h` | none | a cycle of `h` | The ring stood on its end. |
| `2×h`, `w×2` | a cycle of 2 on that axis | | Legal and it works, but **one hop is already a return trip** on that axis, and one dark slot leaves the survivor with nothing to route around to. |
| `3×2` | a real cycle of 3 | a cycle of 2 | **The smallest honest two-axis map**, and M4's target rig. |

**The consequence an operator must expect, stated plainly:** on an axis of length 2, killing
one of the two peers closes the survivor's export edge on that axis with `no_peer`. It does
not re-pair, because there is no third slot to re-pair to. On the 3×2 rig this means a dead
slot in row 1 leaves its column partner with an open **east** lane (its row still holds three
slots, two of them deliverable) and a closed **north** lane. That is route-around working
correctly against a degenerate axis, not route-around failing — and it is why a taller map is
the answer for a rig that wants the north lane to survive a kill.

---

## 3. Transport

| Property | Value |
|---|---|
| Protocol | WebSocket (RFC 6455) over plain HTTP. TLS is **M5** (D9, D16). |
| URL | `ws://{relay-host}:{port}/contract-b/v3` |
| Default port | `8790` |
| Bind address | The relay binds a LAN-reachable address, **not** loopback. The operator opens the Windows Firewall rule for the port and records the host name in `dev_environment.md`. |
| Roles | The **relay is the server**. Every sidecar and the archive are **clients** and do all the dialling. |
| Frame type | Text frames. One JSON object per frame. No batching. |
| Encoding | UTF-8, no BOM |
| Compression | `permessage-deflate` **MUST NOT** be negotiated |
| Max frame size | 8 MiB (`maxFrameBytes`), same as Contract A |
| Authentication | A **shared bearer token** on the HTTP upgrade — §3.1. Unchanged from M3. |
| Reconnect | Exponential backoff with full jitter, `relayBackoffMinMs` to `relayBackoffMaxMs`, the same rule Contract A §6.2 gives the mod. The ladder resets only after a session that stayed up for `stableSessionMs` (`contract-a.md` §13, A8). |

**The path moved with the major**, from `/contract-b/v2` to `/contract-b/v3`. A relay
**MUST** keep serving `/contract-b/v2` and **MUST** close every connection on it immediately
with `4000`, so an M3 sidecar gets the defined loud error instead of a bare HTTP 404. The
same rule, and the same reason, as `contract-a.md` §15, A23.

### 3.1 The LAN token

Unchanged from M3 in every particular. It is restated here in full because this document
inherits nothing silently.

| Rule | Statement |
|---|---|
| Where | The HTTP request that opens the WebSocket carries `Authorization: Bearer <token>`. Nothing token-related appears in any frame. |
| Who | Every client: all six sidecars and the archive. One token for the whole map. |
| Source | The environment variable `MULTIVERSE_TOKEN`, or `--token-file <path>` which reads the first line and strips trailing whitespace. A flag that takes the token literally **MUST NOT** exist — it would put the secret in every process listing. |
| Shape | 16 to 256 bytes of printable ASCII. The RECOMMENDED value is 32 random bytes, hex-encoded. |
| Comparison | Constant-time (`crypto/subtle.ConstantTimeCompare`). Never `==`. |
| Missing or wrong | The relay answers HTTP **401** with `WWW-Authenticate: Bearer` and **does not upgrade**. There is no WebSocket, so there is no close code. |
| No token configured | The relay **MUST** refuse to start, unless `--insecure-no-token` is passed, in which case it logs one loud warning per accepted connection. The flag exists for a single-machine test rig and is never used on the LAN. |
| Client reaction to 401 | Retry on the normal backoff ladder, and log one loud error each time. After `authFailuresBeforeCeiling` (5) consecutive 401s, hold the backoff at `relayBackoffMaxMs`: a wrong token is an operator problem and hammering the relay will not fix it. |

**What this does and does not buy.** It keeps an unrelated device on the LAN out of the map.
It does **not** authenticate a peer *identity*: a token holder can present any `peerId`,
including one that already holds a slot, and the `4006` rule (§3.2) will then evict the
legitimate peer. It does **not** provide confidentiality. Both gaps are accepted for a
milestone whose entire network is two computers the owner owns, and both are closed by M5's
TLS and per-peer credentials (§13 item 1).

**One M4 rule sharpens the identity gap, and it is worth naming here.** Slot handover (§7.5)
rebinds a reservation to a new `peerId` by operator command, on the relay's own machine. It
is not a wire operation and no frame can trigger it — which is deliberate, because on this
transport a `peerId` is a claim, not a credential.

### 3.2 Close codes

| Code | Name | Sent by | Meaning |
|---|---|---|---|
| `1000` | `NORMAL` | either | Clean shutdown. |
| `1009` | `TOO_BIG` | either | Frame over `maxFrameBytes`. |
| `4000` | `PROTOCOL_UNSUPPORTED` | relay | The `protocol` **major** version is not supported, or the connection arrived on a retired path (§3). The client **MUST NOT** reconnect until it is restarted. |
| `4003` | `MALFORMED_FRAME` | either | Not valid JSON, a missing REQUIRED envelope field, no `HANDSHAKE` first, or a routing field that disagrees with the sender's own peer id. Reconnect with backoff. |
| `4004` | `LIVENESS_TIMEOUT` | relay | No frame and no `PONG` within `peerTimeoutMs`. Reconnect with backoff. |
| `4005` | `SHUTTING_DOWN` | either | The sender is draining. Reconnect with backoff. |
| `4006` | `REPLACED` | relay | A newer connection claimed the same `peerId`. The old connection **MUST NOT** reconnect. |

---

## 4. The envelope

Identical in shape to Contract A §3 — five fields, no more:

```json
{
  "protocol": "contract-b/3.0",
  "type": "MIGRATION_PAYLOAD",
  "messageId": "b7d1e0c4-9f2a-4c31-8b6d-2e0a41f5c7a9",
  "sentAt": 1785693600123,
  "data": { }
}
```

`contract-a.md` §3.1 and §3.2 apply unchanged: the version segment is `<major>.<minor>`,
compatibility is on the **major** only, the minor is never a rejection reason, changes within
a major are additive fields and additive enum values only, unknown fields and unknown types
are ignored, and a malformed frame closes with `4003`.

**This is a major bump from M3's `contract-b/2.0`**, and the contract's own test says so
twice over:

| M4 change | Kind | Needs a major? |
|---|---|---|
| `SECTOR_GRANT.eastNeighbour` replaced by `neighbours`, keyed by export edge | field removal + type change | **yes** |
| `SECTOR_CLAIM.exportEdge` (string) becomes `exportEdges` (array) | field removal + type change | **yes** |
| `ringSize` renamed `slotCount` throughout | field removal | **yes** |
| `MIGRATION_NACK.SLOT_VACANT` reclassified from transient to permanent | a `class` value changes meaning for an existing code | **yes**, and it is the dangerous one — see §6.8 |
| `SECTOR_CLAIM.preferredPosition`, `insertAfterSlot`, `stats` added | additive | no |
| `PEER_STATUS.slots[].position`, `stats`, `darkSinceMs`; `map` | additive | no |
| `MIGRATION_NACK.neverForwarded`, `relaySessionId`; codes `PEER_OFFLINE`, `NOT_FORWARDED` | additive field, additive enum values | no |
| `MIGRATION_PAYLOAD.reroute` added; `exitEdge` may now be `"N"` | additive field, existing enum value | no |

A `contract-b/2` sidecar and a `contract-b/3` relay are incompatible by design and say so
with close `4000` instead of misrouting an organism.

Timestamps are informational (D5). `messageId` is for log correlation only; `migrationId` is
the one idempotency key in the system (`contract-a.md` §7.1).

---

## 5. Routing, and what the relay may read

The relay reads exactly two fields out of `data` and nothing else:

| Field | Present on | Routes to |
|---|---|---|
| `destSlot` | `MIGRATION_PAYLOAD` | The peer that currently holds that slot |
| `destPeer` | `MIGRATION_ACK`, `MIGRATION_NACK`, `GENOME_REQUEST`, `GENOME_RESPONSE` | That peer id |

Every routable frame also carries `sourcePeer`. The relay **MUST** check it against the
sending connection's own peer id and **MUST** close with `4003` when they disagree. It
**MUST NOT** rewrite it, so the frame can be forwarded byte for byte.

The relay **MUST NOT** decode `data.body.bb8`, **MUST NOT** decode `data.lineage`, and
**MUST NOT** validate a payload (D4 — that is `bb8-schema`'s job, sidecar-side, at both
ends). It forwards the original frame bytes unchanged.

**Routing is on the slot, not on the peer, and not on the position.** `destSlot` is an
address: if the peer holding that slot changed identity since the sender journaled the
migration, or if the map moved that slot to a different coordinate, the frame goes to whoever
holds the slot **now**. This is what makes insertion, re-positioning and handover safe for
work in flight (§7.3), and it is why the coordinate never appears on a migration frame.

When the destination slot has no live peer, or `destPeer` is not connected, the relay
**MUST** answer the *sender* — `MIGRATION_NACK` with `SLOT_VACANT`, `PEER_OFFLINE` or
`NOT_FORWARDED` for a migration, `GENOME_RESPONSE` with `found: false,
reason: "peer_offline"` for a fetch — rather than drop the frame. A dropped frame turns a
bounded failure into a stall, and under M4 it also withholds the evidence a sender needs to
re-route (§5.2).

**The relay computes the effective neighbour, and that is the only new thinking it does**
(D12). Walking a row or a column over the registry it already holds adds no new knowledge to
a deliberately dumb relay: it still parses no body, indexes nothing, and stores no
organism. §8 states the walk.

### 5.1 The archive is a read-only subscriber

`multiverse-archive` connects to the relay as a client with `role: "archive"` (§6.1). It
owns no world, holds no slot, and never appears in the structural order.

| Rule | Statement |
|---|---|
| Fan-out | The relay **MUST** send every connected subscriber a **byte-identical copy** of every `MIGRATION_PAYLOAD` it routes, and of every `MIGRATION_ACK` and `MIGRATION_NACK` it routes. The copy carries the original `sourcePeer`, `destSlot` and `migrationId`. |
| Best effort | The fan-out **MUST NOT** delay, block or fail a migration. A subscriber that is absent, slow or dead changes nothing on the migration path. |
| Bounded | Each subscriber has a queue of `archiveQueueMax` (1024) frames. On overflow the relay **MUST** drop the **oldest** copy, increment a dropped-copies counter, and log at most one line per minute. It **MUST NOT** disconnect the migration it was copying. |
| No answers | A subscriber **MUST NOT** answer a copied frame. The relay ignores an `ACK`/`NACK` from a subscriber with one warning. |
| No sending | A `MIGRATION_PAYLOAD` from a subscriber is answered `MIGRATION_NACK` / `NOT_A_MEMBER` and is **not** forwarded. |
| What a subscriber may send | `HANDSHAKE`, `PING`, `PONG`, `GENOME_REQUEST`. Nothing else. |
| No claim | A `SECTOR_CLAIM` from a subscriber is refused with `granted: false, reason: "role_has_no_slot"`. |
| Duplicates | A re-forwarded or re-routed migration produces a second copy. The archive deduplicates on `migrationId`, exactly as a sidecar does — and a **re-routed** copy carries a different `destSlot` with the same `migrationId`, which is not a duplicate organism but the same organism on a new lane (§6.6, §9). |
| Full state, no polling | A subscriber also receives every `PEER_STATUS` broadcast, which is what lets the status page render the whole map without asking anybody anything (§10.1). |

The M3 reasoning for the copy over a slot-less ring member is unchanged and still holds: a
slot-less member has to be special-cased in every rule that says "each peer has neighbours",
in the placement protocol and in the ripple; the copy is one rule in the router; and the
archive stays outside the migration path, so nothing in a migration ever waits for it.

### 5.2 The forwarding record — what makes a non-delivery provable

**New in M4 (D12).** Route-around introduces a second possible destination for one organism,
and D2 forbids duplication. The sender may re-route a journaled hop **only** under proof that
no custody was ever taken, and the only party that can prove it is the relay: a frame it
never forwarded reached no sidecar and created no custody.

| Rule | Statement |
|---|---|
| The record | The relay keeps the set of `migrationId`s it has **forwarded**, with the time of the first forward, for `forwardRecordRetentionSeconds` (172 800 s = 48 h). |
| What counts as forwarded | **Any attempted write of the frame to a destination peer's connection**, whether or not that write later fails. A partial write and a peer that dies with bytes in its receive buffer are indistinguishable from a complete delivery, so both count as forwarded. |
| What does not count | A frame the relay refused before writing: no such slot, no live peer for that slot, the destination's outbound queue full, the relay draining. |
| The session | The relay mints a `relaySessionId` (a UUID) at process start and reports it in `HANDSHAKE_ACK` (§6.2) and in every relay-generated `MIGRATION_NACK` (§6.8). The record covers **that session only**. |
| The answer | A relay-generated `MIGRATION_NACK` carries `neverForwarded: true` **only** when the `migrationId` is absent from the record of the current session. Otherwise it carries `false`. |
| Memory | One `migrationId` and one timestamp per forwarded migration. At T1's measured rate — 1 799 hops an hour — 48 hours is about 86 000 entries, a few megabytes. It is in memory, and it is deliberately **not** durable: a relay restart is exactly the event that invalidates the proof, and persisting it would claim knowledge the new process does not have. |

**Why the session id, and not a timestamp.** The sender has to know whether the relay's
answer covers the whole life of its journal entry. A timestamp comparison would put a
correctness decision on another machine's clock, which D5 forbids. The session id makes the
test exact and clock-free: the sender records the `relaySessionId` under which the entry was
first handed to a live relay connection, and the proof counts only when the two ids match
(§9.2). A **link flap** keeps the id and keeps the proof; a **relay restart** changes it and
the sender falls back to holding.

**The failure direction is deliberate.** Every ambiguity resolves toward `neverForwarded:
false` — toward holding, toward waiting, toward the bounded hold and eventually the bounce —
because holding costs a delay and re-routing on a bad proof costs a duplicated organism. Risk
2 names the implementation that gets this wrong: one that treats **silence** as proof.
Silence is never proof in this contract; only a relay statement or a peer NACK is.

---

## 6. Message catalogue

Twelve types. **None is new in M4** — the non-delivery proof rides on the existing
`MIGRATION_NACK`, and the operator commands are deliberately not wire operations (§7.5).

| Type | Direction | Answered by |
|---|---|---|
| `HANDSHAKE` | client → relay | `HANDSHAKE_ACK`, or a close |
| `HANDSHAKE_ACK` | relay → client | nothing |
| `SECTOR_CLAIM` | sidecar → relay | `SECTOR_GRANT` |
| `SECTOR_GRANT` | relay → sidecar | nothing |
| `PEER_STATUS` | relay → client | nothing |
| `MIGRATION_PAYLOAD` | sidecar → sidecar, forwarded; copied to subscribers | `MIGRATION_ACK` or `MIGRATION_NACK` |
| `MIGRATION_ACK` | sidecar → sidecar, forwarded; copied | nothing |
| `MIGRATION_NACK` | sidecar → sidecar or relay → sidecar; copied | nothing |
| `GENOME_REQUEST` | archive or sidecar → sidecar, forwarded | `GENOME_RESPONSE` |
| `GENOME_RESPONSE` | sidecar → requester, forwarded; relay → requester on failure | nothing |
| `PING` / `PONG` | either | `PONG` / nothing |

The two claim messages keep their M2 names for the third milestone running. The noun changed
twice — a sector became a ring slot, and a ring slot became a slot with a position — but
`system_decomposition.md`'s ratified Contract B message list still names `SECTOR_CLAIM`, and
renaming a ratified message to chase a noun buys less than it costs. **The fields are renamed
wherever the noun was wrong**, which is the rule M3 set and M4 follows: `ringSize` becomes
`slotCount`, `eastNeighbour` becomes `neighbours`, and nothing inside a frame says "ring" or
"sector" any more. §11 item 1 records the choice.

### 6.1 `HANDSHAKE` — client → relay

The **first frame on every connection**. Any other first frame closes with `4003`.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `peerId` | string | yes | Stable identity of this client. `1`–`64` characters, `[A-Za-z0-9._-]`. It is what makes a slot reclaim work across a restart, so it **MUST** be persisted (§7.4). |
| `role` | string enum | yes | `"peer"` — owns a world and a slot — or `"archive"` — a read-only subscriber (§5.1). |
| `protocolVersion` | string | yes | `"contract-b/3.0"`. A different **major** closes with `4000`. |
| `gameVersion` | string | yes | The game version behind this sidecar, from the mod's `CONFIG_UPDATE`. Empty while no mod is connected, and always empty for an archive. |
| `sidecarVersion` | string | yes | Informational. The archive sends its own version here. |
| `simulationSize` | float | no | `S`, when a mod has already reported one. |

A second connection presenting a `peerId` that is already live **MUST** cause the relay to
close the older connection with `4006` and serve the newer one — the same self-healing rule
`contract-a.md` §2 gives the mod socket.

**Compatibility enforcement at connect.** The relay **MUST** refuse a peer whose
`gameVersion` is incompatible with the map's, and it **MUST** be loud about it: it closes
with `4003`, logs one error naming both versions, and reports the refusal in the next
`PEER_STATUS` as `lastRefusal` on that slot. A silent version mismatch is indistinguishable
from a dead peer — under M4 both end with a lane routed around them — and M4 crosses two
independently updated installs, so this is the failure most likely to waste an evening. An
**empty** `gameVersion` is not a mismatch: it means no mod is connected yet.

```json
{
  "protocol": "contract-b/3.0",
  "type": "HANDSHAKE",
  "messageId": "9d1a4b77-2c60-4c1e-9f03-77a1c8e4b510",
  "sentAt": 1785693597011,
  "data": {
    "peerId": "peer-lan-slot5",
    "role": "peer",
    "protocolVersion": "contract-b/3.0",
    "gameVersion": "0.6.3.1",
    "sidecarVersion": "0.4.0",
    "simulationSize": 2000.0
  }
}
```

### 6.2 `HANDSHAKE_ACK` — relay → client

| Field | Type | Required | Semantics |
|---|---|---|---|
| `relayVersion` | string | yes | Informational. |
| `protocolVersion` | string | yes | `"contract-b/3.0"`. |
| `relaySessionId` | `uuid` | yes | **New in M4.** Minted once at relay start, constant for the life of the relay process. It is the scope of the forwarding record (§5.2), and a sidecar **MUST** persist it against every journal entry it hands over while this connection is live (§9.2). |
| `assignedSlot` | number (int) | no | The slot this `peerId` already holds, when the relay remembers one. Absent for a first-time peer and always absent for an archive. |
| `assignedPosition` | object `{col,row}` | no | Its position. Present exactly when `assignedSlot` is. |
| `map` | object `{width,height}` | yes | The rectangle right now. `{"width":0,"height":0}` before the first placement. |
| `slotCount` | number (int) | yes | How many slots are reserved right now. `0` before the first placement. Renamed from M3's `ringSize`. |
| `receivedAt` | `timestampMs` | yes | The relay's own clock. Informational, and the anchor the archive uses to order records written by six machines' clocks. |

```json
{
  "protocol": "contract-b/3.0",
  "type": "HANDSHAKE_ACK",
  "messageId": "0b4e2a13-5d77-4b90-8a21-6f0c19d4e772",
  "sentAt": 1785693597019,
  "data": {
    "relayVersion": "0.4.0",
    "protocolVersion": "contract-b/3.0",
    "relaySessionId": "5f0b9c31-77ad-4e26-9a4c-1b83d206ef95",
    "assignedSlot": 5,
    "assignedPosition": { "col": 1, "row": 1 },
    "map": { "width": 3, "height": 2 },
    "slotCount": 6,
    "receivedAt": 1785693597018
  }
}
```

### 6.3 `SECTOR_CLAIM` — sidecar → relay

A **placement claim**. Sent right after `HANDSHAKE`, and again whenever `simulationSize`,
`exportEdges`, `gameVersion` or `modConnected` change. A repeat claim from a peer that
already holds a slot is an **update**, never a second claim.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `preferredSlot` | number (int) | no | The slot this sidecar held last time, replayed from `<data-dir>/slot` (§7.4). Advisory. `≥ 1`. |
| `preferredPosition` | object `{col,row}` | no | **New in M4.** An advisory position. It may name a **hole** inside the current rectangle, or a position exactly one column or one row **outside** it, which asks the relay to extend the map on that axis (§7.2 rule 4). Both coordinates `≥ 0`. |
| `insertAfterSlot` | number (int) | no | **New in M4.** An advisory splice: "place me immediately after this slot on `insertAxis`". The relay inserts a column (`"E"`) or a row (`"N"`) after that slot's own column or row and places the newcomer at the crossing (§7.2 rule 5). |
| `insertAxis` | edge enum | no | `"E"` or `"N"`. Default `"E"`. Meaningful only with `insertAfterSlot`. |
| `simulationSize` | float | yes | `S`, as last reported by the mod. `0` while no mod is connected. |
| `exportEdges` | array of edge | yes | The mod's declared export edges, from Contract A `CONFIG_UPDATE` (`contract-a.md` §5.1, §15 A18). `["E","N"]` under the grid. `[]` while no mod is connected. Replaces M3's singular `exportEdge`. |
| `borderEdges` | array of edge | yes | The mod's declared strips — the edges that accept an inbound organism. `["E","N","W","S"]` under the grid, `[]` while no mod is connected. |
| `gameVersion` | string | no | Updates the value from `HANDSHAKE`. |
| `modConnected` | bool | yes | Whether a mod is connected right now. A sidecar with no mod cannot spawn an organism, so it is not **deliverable** and every lane pointed at it re-pairs around it (§8). |
| `stats` | object | no | The peer stats block of §6.3.1. Present on any claim where a value is known. |

**The claim is advisory in every part, and it never fails for a lost race** (§7.2). A
position that is taken, a splice after a slot that does not exist, or two peers asking for
one hole all resolve to *some* placement, and the grant names the placement the peer actually
received. There is no `position_taken` refusal, because a refusal would leave a peer with no
world in the map and nothing useful to do about it.

```json
{
  "protocol": "contract-b/3.0",
  "type": "SECTOR_CLAIM",
  "messageId": "4c7f0d92-8a11-4e63-bb05-2d971a0c3e44",
  "sentAt": 1785693597033,
  "data": {
    "preferredSlot": 5,
    "simulationSize": 2000.0,
    "exportEdges": ["E", "N"],
    "borderEdges": ["E", "N", "W", "S"],
    "gameVersion": "0.6.3.1",
    "modConnected": true,
    "stats": {
      "population": 214,
      "eggCount": 37,
      "custodyDepth": 2,
      "pacedDepth": 0,
      "heldDepth": 0,
      "lastSave": {
        "atMs": 1785693540118,
        "simulatedTime": 119303.50,
        "population": 211,
        "name": "M4-Slot5-20260805T2058Z.zip",
        "bytes": 41533892,
        "durationMs": 730
      }
    }
  }
}
```

A brand-new instance asking to extend a full 3×2 map into a fourth column — the exit test's
Part 4, in one frame:

```json
{
  "protocol": "contract-b/3.0",
  "type": "SECTOR_CLAIM",
  "messageId": "8e3a05c7-19bd-4f42-a0e6-72c4198bd3f0",
  "sentAt": 1785694011500,
  "data": {
    "preferredPosition": { "col": 3, "row": 0 },
    "simulationSize": 2000.0,
    "exportEdges": ["E", "N"],
    "borderEdges": ["E", "N", "W", "S"],
    "gameVersion": "0.6.3.1",
    "modConnected": true,
    "stats": { "population": 0, "custodyDepth": 0, "pacedDepth": 0, "heldDepth": 0 }
  }
}
```

### 6.3.1 The peer stats block

**New in M4 (D15).** One shape, three carriers: a sidecar sends it on `SECTOR_CLAIM` (§6.3)
and on `PING` (§6.11), and the relay republishes the latest value it holds in `PEER_STATUS`
(§6.5). It exists so the operator surface can describe **six** worlds — two of them on a
machine the archive cannot read a file from — without anything reading anything else's
memory (Risk 4).

| Field | Type | Required | Semantics |
|---|---|---|---|
| `population` | number (int) | no | Live organisms in this peer's world, from `HEARTBEAT.population` (`contract-a.md` §5.2). Absent when no mod is connected — **absent means unknown, and a reader MUST NOT render it as zero**. |
| `eggCount` | number (int) | no | Live eggs, when the mod reports them. |
| `custodyDepth` | number (int) | no | Journal entries this sidecar holds custody of right now: outbound entries awaiting `MIGRATION_ACK` plus inbound entries awaiting `MIGRATE_IN_ACK`. |
| `pacedDepth` | number (int) | no | Inbound entries waiting on the delivery rate limit (`contract-a.md` §7.5). A depth that never falls names a limit set too low. |
| `heldDepth` | number (int) | no | Outbound entries in the **held** state of §9.2 — forwarded, unproven, destination dark. |
| `bouncedTimeoutTotal` | number (int) | no | Cumulative count of entries this sidecar has bounced home because the hold timeout expired. A monotonic counter, reset only by losing the journal. **An automatic bounce is a fact the operator reads, not a silent repair** (§9.3). |
| `simulatedTime` | float | no | The world's simulated seconds, from the last `HEARTBEAT`. It is what makes a paced rate interpretable. |
| `lastSave` | object | no | The mod's save receipt, copied verbatim from `HEARTBEAT.lastSave` (`contract-a.md` §5.2, §15 A21): `atMs`, `simulatedTime`, `population`, and the optional `name`, `bytes`, `durationMs`. |

**Every field is optional, and absence is a value.** A stat the sidecar does not know is
omitted, never defaulted. The status page renders an omitted field as **unknown**, which is
Risk 4's rule: a slot that reports nothing is unknown, not empty, and an honest gap beats a
confident zero.

**The relay does not interpret any of it.** It stores the last block per peer with the time
it arrived, republishes it, and never routes, schedules, refuses or filters on a stat. D1's
dumb relay survives: this is one more field copied into a broadcast it was already sending.

### 6.4 `SECTOR_GRANT` — relay → sidecar

The grant returns the slot, the position, the map, and **one effective neighbour per export
edge**. Together they are the entire topology a sidecar needs (D8, D12, D13).

| Field | Type | Required | Semantics |
|---|---|---|---|
| `granted` | bool | yes | |
| `slot` | number (int) | no | Present when `granted` is `true`. The routing address. Never reused, never renumbered. |
| `position` | object `{col,row}` | no | Present when `granted` is `true`. **May change** across grants when the map grows (§7.3). |
| `map` | object `{width,height}` | yes | The rectangle after this grant. |
| `slotCount` | number (int) | yes | Reserved slots after this grant. |
| `reason` | string enum | yes | `"granted"` (a new slot was placed), `"reclaimed"` (the reservation for this `peerId` was still held), `"updated"` (a repeat claim), `"repositioned"` (same slot, new coordinate, because the map grew), `"handover"` (this peer inherited a slot by operator command — §7.5), `"role_has_no_slot"`, `"protocol_mismatch"`, `"version_incompatible"`. |
| `neighbours` | object | no | Present when `granted` is `true`. Keyed by **export edge**: `"E"`, `"N"`. **A key is absent when that axis has no deliverable target**, and its absence is what closes that export edge with `no_peer` (§8). |
| `neighbours.<edge>.slot` | number (int) | yes | The **effective** target: the first deliverable slot along that axis. |
| `neighbours.<edge>.peerId` | string | yes | The identity reserved to that slot. |
| `neighbours.<edge>.position` | object `{col,row}` | yes | Its coordinate. Informational for the sidecar; useful in a log line that has to be read by a human looking at a map. |
| `neighbours.<edge>.live` | bool | yes | Always `true` — a target is deliverable by construction. Carried so a log line is self-describing. |
| `neighbours.<edge>.modConnected` | bool | yes | Always `true`, for the same reason. |
| `neighbours.<edge>.gameVersion` | string | yes | For the compatibility test. |
| `neighbours.<edge>.simulationSize` | float | yes | For the `S` test of `contract-a.md` §13, A10. |
| `neighbours.<edge>.skipped` | array of object | yes | **The bypass list**, in walk order. `[]` when the lane is direct. Each entry: `{"slot": int\|null, "position": {col,row}, "reason": string}`. `slot` is `null` for a **hole** — a position with no reservation at all. |
| `neighbours.<edge>.skipped[].reason` | string enum | yes | `"hole"`, `"peer_offline"`, `"peer_mod_absent"`, `"peer_incompatible"`, `"sim_size_mismatch"`. These are the M3 edge-close reasons, demoted: each one now skips a slot instead of closing a lane (D12). |

A refused claim carries `granted: false` and no `slot`. The sidecar keeps its export edges
closed and retries on the next `PEER_STATUS` change.

**A grant is sent whenever any of its content changes**, which under route-around means
whenever a peer's effective neighbour on either axis changes — not only when the peer claims.
§7.2 fixes the ordering that makes those broadcasts consistent.

```json
{
  "protocol": "contract-b/3.0",
  "type": "SECTOR_GRANT",
  "messageId": "e2b90c47-1f35-4d02-9c68-51a7d3b0f981",
  "sentAt": 1785693731655,
  "data": {
    "granted": true,
    "slot": 4,
    "position": { "col": 0, "row": 1 },
    "map": { "width": 3, "height": 2 },
    "slotCount": 6,
    "reason": "updated",
    "neighbours": {
      "E": {
        "slot": 6,
        "peerId": "peer-main-slot6",
        "position": { "col": 2, "row": 1 },
        "live": true,
        "modConnected": true,
        "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0,
        "skipped": [
          { "slot": 5, "position": { "col": 1, "row": 1 }, "reason": "peer_offline" }
        ]
      },
      "N": {
        "slot": 1,
        "peerId": "peer-main-slot1",
        "position": { "col": 0, "row": 0 },
        "live": true,
        "modConnected": true,
        "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0,
        "skipped": []
      }
    }
  }
}
```

That is slot 4 one second after slot 5 was hard-killed: its east lane skipped the dark slot
and re-paired to slot 6 **without closing**, and its north lane — which wraps from row 1 back
to row 0 in its own column — never noticed. Compare it with slot 2, whose column holds only
slot 5 besides itself: that peer's grant carries **no `"N"` key at all**, and its north edge
closes with `no_peer` (§2.1).

### 6.5 `PEER_STATUS` — relay → client

**Full state, not a delta**, exactly like Contract A's `EDGE_STATUS`. Sent after every
registry change: a peer connecting or dying, a slot granted, released or handed over, a map
growth, a `simulationSize` update, a mod connecting or disconnecting behind a peer. Also sent
on a `statsBroadcastIntervalMs` timer, because stats change without the registry changing.

`PEER_STATUS` reports **the structure, not the effect** (D12). It is the map as it is
reserved; the lanes as they currently run are in each peer's own `SECTOR_GRANT`. Publishing
both is deliberate: a structure alone hides a bypass, and an effect alone hides the shape.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `epoch` | number (int64) | yes | Strictly increasing per connection, from 1. A receiver **MUST** ignore an epoch lower than or equal to the last applied one. Resets on a new connection. |
| `map` | object `{width,height}` | yes | The rectangle. Every `col ∈ [0,width)` and `row ∈ [0,height)` exists as a position. |
| `slotCount` | number (int) | yes | Number of entries in `slots`. Renamed from M3's `ringSize`. |
| `slots` | array of object | yes | **In structural order: `row` ascending, then `col` ascending.** Every reserved slot appears, live or not. |
| `slots[].slot` | number (int) | yes | The routing address. Stable for the life of the reservation. |
| `slots[].position` | object `{col,row}` | yes | Its coordinate in the rectangle. |
| `slots[].peerId` | string | yes | The reserved identity. Never empty — a slot exists only because a peer claimed it, or because an operator handed it over. |
| `slots[].live` | bool | yes | Whether that peer has a live relay connection. |
| `slots[].modConnected` | bool | yes | Whether that peer has a live mod. |
| `slots[].gameVersion` | string | yes | Empty when unknown. |
| `slots[].simulationSize` | float | yes | `0` when unknown. |
| `slots[].exportEdges` | array of edge | yes | What that peer declared. `[]` when unknown. It is what tells a reader which lanes that peer is even trying to run. |
| `slots[].lastSeenMs` | `timestampMs` | no | Relay clock, last frame from that peer. Informational, for the operator. |
| `slots[].darkSinceMs` | `timestampMs` | no | **New in M4.** Relay clock, the moment this peer's connection was lost. Present exactly when `live` is `false` and the relay saw it go. Absent for a peer that has never connected in this relay session. This is the field Risk 5 needs: **a healed map hides a dead world**, and "bypassed since 04:12" is what stops an operator missing it for a day. |
| `slots[].lastRefusal` | string | no | Present when the relay last refused this peer's connection, naming the reason (§6.1). This is how a version mismatch stops looking like a dead peer. |
| `slots[].stats` | object | no | The last peer stats block from that peer (§6.3.1). Absent when none has arrived. |
| `slots[].statsAsOfMs` | `timestampMs` | no | Relay clock when that block arrived. Present exactly when `stats` is. **A reader MUST use it to age the stats**: a population from a peer that went dark an hour ago is history, not state. |
| `you` | object | yes | `{"slot": int\|null, "position": {col,row}\|null, "neighbours": {"E": int\|null, "N": int\|null}}` for the receiving client. All null for a subscriber. |
| `observers` | number (int) | yes | Connected read-only subscribers. Informational. |

**Holes are derived, not sent.** A position is a hole when it is inside the rectangle and no
entry in `slots` names it:

```
holes = { (c, r) : 0 ≤ c < map.width, 0 ≤ r < map.height }
      − { s.position : s ∈ slots }
```

Sending them would be a second copy of a fact already on the wire, and a second copy is a
second thing to get out of step.

**Any client can reproduce the effective lanes from this frame**, and that is what lets the
archive draw the whole map — bypasses included — without asking six sidecars anything. The
walk of §8 is deterministic and its every input is here: `position`, `live`, `modConnected`,
`gameVersion`, `simulationSize`, `exportEdges`. **The relay's `SECTOR_GRANT` is still the
authority** for a peer's own routing; a subscriber's recomputation is for display, and where
the two disagree the display is stale.

```json
{
  "protocol": "contract-b/3.0",
  "type": "PEER_STATUS",
  "messageId": "77c0e1a4-63b8-4f19-8d2a-9e40b7c15206",
  "sentAt": 1785693731650,
  "data": {
    "epoch": 41,
    "map": { "width": 3, "height": 2 },
    "slotCount": 6,
    "slots": [
      { "slot": 1, "position": { "col": 0, "row": 0 }, "peerId": "peer-main-slot1",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731644,
        "stats": { "population": 231, "custodyDepth": 1, "pacedDepth": 0, "heldDepth": 0 },
        "statsAsOfMs": 1785693731644 },
      { "slot": 2, "position": { "col": 1, "row": 0 }, "peerId": "peer-main-slot2",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731641,
        "stats": { "population": 208, "custodyDepth": 0, "pacedDepth": 0, "heldDepth": 0 },
        "statsAsOfMs": 1785693731641 },
      { "slot": 3, "position": { "col": 2, "row": 0 }, "peerId": "peer-main-slot3",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731639,
        "stats": { "population": 197, "custodyDepth": 0, "pacedDepth": 0, "heldDepth": 0 },
        "statsAsOfMs": 1785693731639 },
      { "slot": 4, "position": { "col": 0, "row": 1 }, "peerId": "peer-lan-slot4",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731630,
        "stats": { "population": 244, "custodyDepth": 3, "pacedDepth": 11, "heldDepth": 2 },
        "statsAsOfMs": 1785693731630 },
      { "slot": 5, "position": { "col": 1, "row": 1 }, "peerId": "peer-lan-slot5",
        "live": false, "modConnected": false, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693719004,
        "darkSinceMs": 1785693719004,
        "stats": { "population": 226, "custodyDepth": 0, "pacedDepth": 0, "heldDepth": 0 },
        "statsAsOfMs": 1785693718991 },
      { "slot": 6, "position": { "col": 2, "row": 1 }, "peerId": "peer-main-slot6",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731647,
        "stats": { "population": 189, "custodyDepth": 0, "pacedDepth": 0, "heldDepth": 0 },
        "statsAsOfMs": 1785693731647 }
    ],
    "you": { "slot": 4, "position": { "col": 0, "row": 1 },
             "neighbours": { "E": 6, "N": 1 } },
    "observers": 1
  }
}
```

In that example slot 5 is reserved, positioned and dark since `1785693719004`. Its row
re-paired around it — slot 4 now exports east to slot 6 — and its **column partner slot 2 has
no north lane at all**, because a column of two holds nobody else to skip to (§2.1). Slot 5's
own stats are 13 seconds stale and `statsAsOfMs` says so; a reader that renders that
population as current is reporting a world that is no longer running.

### 6.6 `MIGRATION_PAYLOAD` — sidecar → sidecar, forwarded

The Contract C `MigrationEnvelope`, carried in `data`, with the lineage annex (D11).

| Field | Type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The idempotency key (D2). Minted by the origin **mod**, preserved end to end — **including across a re-route** (§9.2). |
| `kind` | string enum | yes | `"bibite"` in M4. Anything else is answered `MIGRATION_NACK` / `KIND_UNSUPPORTED`. |
| `body.version` | string | yes | The game version that serialized the blob. Authoritative over the blob's own `version` key (`contract-a.md` §4.6). |
| `body.bb8` | string | yes | The opaque blob, as a JSON **string**, never nested, never base64. Max `maxPayloadBytes` (4 MiB). |
| `lineage` | object | yes | The annex. Always present; `parents` may be empty. |
| `lineage.genomeHash` | string | yes | The migrant's own genome hash, computed by the **source sidecar** from `body.bb8` with `genome-hash.md`. The archive's join key. **The empty string when the migrant's own genome will not hash** — see below. Always present as a key. |
| `lineage.parents` | array of object | yes | `0`–`2` entries, in `genes.parent1` then `genes.parent2` order. `[]` is normal. |
| `lineage.parents[].entityId` | `entityId` | yes | The parent's id, from the migrant's genes. Signed int32, often negative. |
| `lineage.parents[].genomeHash` | string | no | The parent's genome hash. **Absent means a gap** — the parent genome was not available to hash. |
| `lineage.parents[].gapReason` | string enum | no | Present exactly when `genomeHash` is absent: `"parent_gone"` (no blob was shipped — the usual case), `"blob_invalid"` (`bb8-schema` could not hash it), `"blob_dropped_for_size"` (the mod trimmed it to fit the frame). **`"blob_dropped_for_size"` is still unreachable** — see below. |
| `sourcePeer` | string | yes | Origin peer id. The relay verifies it. |
| `sourceSlot` | number (int) | yes | The origin's slot. |
| `destSlot` | number (int) | yes | The origin's **effective neighbour on `exitEdge`** at the moment the migration was journaled, or the slot a re-route redirected it to (§9.2). The relay routes on this and on nothing else. |
| `exitEdge` | edge enum | yes | `"E"` or `"N"` under the grid (amended for M4). It is the axis the organism left by, and it is what the receiver turns into an entry edge. |
| `exitPosition` | float | yes | `[0,1]` along that edge, by `contract-a.md` §4.3. Already clamped by the origin mod — the capture band makes an unclamped raw value routine (`contract-a.md` §4.3.1). |
| `velocity` | `{x,y}` | yes | Copied, never mirrored (`contract-a.md` §4.4). |
| `heading` | float | yes | Degrees (`contract-a.md` §4.4). |
| `entityId` | `entityId` | yes | Signed int32, often negative. See §11 item 3 for which value wins. |
| `timestamp` | `timestampMs` | yes | The origin's clock. Informational (D5). |
| `reroute` | object | no | **New in M4.** Present exactly when this frame's `destSlot` is not the one the entry was first journaled with (§9.2). Informational: the relay does not read it and the receiver does not act on it. It is what lets the archive and the status page say *why* an organism took the lane it took. |
| `reroute.fromSlot` | number (int) | yes | The `destSlot` this entry originally carried. |
| `reroute.count` | number (int) | yes | How many times this entry has been re-routed, starting at 1. Bounded by `maxReroutes` (§9.2). |
| `reroute.proof` | string enum | yes | What authorized it: `"never_sent"`, `"relay_never_forwarded"`, `"peer_refused"`. Nothing else may appear, because nothing else is proof (§9.2). |
| `reroute.atMs` | `timestampMs` | yes | When the sender rewrote the destination. Informational. |

**`entryEdge` is not a field, and the receiver derives it from `exitEdge`** (amended for
M4). Under the grid the derivation is the opposite-edge function of `contract-a.md` §4.2:

| The sender's `exitEdge` | The receiver delivers on `entryEdge` |
|---|---|
| `"E"` | `"W"` — the passive west entry edge |
| `"N"` | `"S"` — the passive south entry edge |
| either, for a **bounce-back** | the **origin's own** `exitEdge`, because a bounce comes home through the door it left by (§9.3) |

`entryPosition` is `exitPosition`, copied. The transverse continuity that D3 buys is
unchanged on both axes: an organism that leaves east at `y` arrives west at the same `y`, and
one that leaves north at `x` arrives south at the same `x` (`contract-a.md` §4.3).

Sending `entryEdge` explicitly would let a sender dictate a receiver's geometry, and the
receiver already knows everything it needs.

**The wire never carries a parent blob.** The mod ships opaque parent blobs on Contract A
(`contract-a.md` §14, A12); the source sidecar hashes them, caches them under their hashes,
and **strips them here**. A genome travels a second time only in answer to a
`GENOME_REQUEST` (§6.9).

**Contract debt — the sidecar can only emit two of the three `gapReason` values.** A parent
the mod dropped for frame size arrives on Contract A as an `entityId` with no `payload`,
which is byte for byte what a dead parent looks like, so the sidecar records `"parent_gone"`
and `"blob_dropped_for_size"` never appears on this wire. The value stays in the enum: the
mod logs each drop on its own side, a receiver must already tolerate every value here, and
removing it would be a wire change for a field a reader must handle defensively anyway.
Closing the debt needs one additive optional flag on Contract A's `parents[]`, and M4 does
not add it either (`contract-a.md` §14 A12, §12 item 8).

The receiving sidecar **MUST**, in this order:

1. Deduplicate on `migrationId` against its journal **and its tombstones**. A hit is
   answered `MIGRATION_ACK` immediately and delivered nothing (`contract-a.md` §7.2). **A
   re-routed frame deduplicates on exactly the same key**, which is what makes a re-route
   that races a late delivery safe: the second arrival is a duplicate, not a second organism.
2. Apply admission control. More than `inboundQueueMax` (64) un-delivered journal entries
   is answered `MIGRATION_NACK` / `OVERLOADED`.
3. Check `S` against its own, by the relative test of `contract-a.md` §13, A10. A mismatch is
   `MIGRATION_NACK` / `SIM_SIZE_MISMATCH`.
4. Validate the blob with `bb8-schema` against `body.version`. Failure is
   `MIGRATION_NACK` / `INVALID_PAYLOAD`.
5. Write the journal entry and **flush it to durable storage**. Custody moves here.
6. Cache the migrant's genome under `lineage.genomeHash`, so this peer can serve it later
   (§10). A cache write failure is logged and never fails the migration.
7. Deliver it to its mod as Contract A `MIGRATE_IN`, **through the delivery rate limit**
   (`contract-a.md` §7.5, §15 A20), replaying until the mod ACKs (`contract-a.md` §7.5).
   Pacing sits **after** step 5, never before it: custody is taken at the speed of the wire
   and released at the speed of the world.

**The receiver never recomputes `lineage.genomeHash` as a gate.** It MAY recompute it for a
consistency log line, but a mismatch is a `bb8-schema` defect to shout about, not a reason
to refuse an organism — custody rules outrank bookkeeping.

**An unhashable migrant sends `genomeHash: ""`, loudly, and still crosses.** `genome-hash.md`
§8 forbids a placeholder hash, so when the source sidecar's own `GenomeHash` call fails there
is nothing to put in the field and it carries the empty string. Three rules follow, and none
of them is optional:

- The source sidecar **MUST** log the failure at error level. A blob that passed
  `bb8-schema` validation and then would not hash is a `bb8-schema` defect, not a bad
  organism, and it is the only signal that defect will emit.
- The source sidecar **MUST** still forward the envelope. It **MUST NOT** substitute a
  placeholder, a truncation, the payload hash, or the hash of a repaired projection.
- The receiver **MUST NOT** treat an empty `genomeHash` as invalid. It is not a
  `MIGRATION_NACK` reason, it does not fail validation, and it does not stop the delivery.

An organism refused at the wire is not recoverable; a gap in the archive is, because the blob
is in the journal and a fixed `bb8-schema` can hash it later. That asymmetry is the whole
argument.

```json
{
  "protocol": "contract-b/3.0",
  "type": "MIGRATION_PAYLOAD",
  "messageId": "1f9c40ab-7d22-4e58-9b31-05c7e2a8d640",
  "sentAt": 1785693600151,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "kind": "bibite",
    "body": {
      "version": "0.6.3.1",
      "bb8": "{\"transform\":{ ... },\"rb2d\":{ ... },\"genes\":{ ... },\"body\":{\"id\":-843827577, ... },\"clock\":{ ... },\"brain\":{ ... },\"version\":\"0.6.3.1\"}"
    },
    "lineage": {
      "genomeHash": "bb8-genome/1:sha256:1725de8f1b61ba91fbeea7c91c47d3060b6ff97afbb6dfc2fc4062879a8bee14",
      "parents": [
        {
          "entityId": -1180911975,
          "genomeHash": "bb8-genome/1:sha256:43ea8315112301799622f3ab8906c2882e528502bc35424a209cd67bfdaec207"
        },
        { "entityId": 204418833, "gapReason": "parent_gone" }
      ]
    },
    "sourcePeer": "peer-lan-slot4",
    "sourceSlot": 4,
    "destSlot": 5,
    "exitEdge": "E",
    "exitPosition": 1.0,
    "velocity": { "x": 61.2, "y": 4.4 },
    "heading": 274.11,
    "entityId": -843827577,
    "timestamp": 1785693600149
  }
}
```

The same organism, re-routed after slot 5 went dark and the relay proved it had never
forwarded the frame. The `migrationId` is the same, the payload is byte-identical, the exit
geometry is untouched, and **only `destSlot` changed** — plus the `reroute` block that says
so:

```json
{
  "protocol": "contract-b/3.0",
  "type": "MIGRATION_PAYLOAD",
  "messageId": "5c07b2e9-84a1-4d36-97f0-1eb3d8a05c74",
  "sentAt": 1785693733120,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "kind": "bibite",
    "body": { "version": "0.6.3.1", "bb8": "{ ... }" },
    "lineage": {
      "genomeHash": "bb8-genome/1:sha256:1725de8f1b61ba91fbeea7c91c47d3060b6ff97afbb6dfc2fc4062879a8bee14",
      "parents": [
        { "entityId": -1180911975,
          "genomeHash": "bb8-genome/1:sha256:43ea8315112301799622f3ab8906c2882e528502bc35424a209cd67bfdaec207" },
        { "entityId": 204418833, "gapReason": "parent_gone" }
      ]
    },
    "sourcePeer": "peer-lan-slot4",
    "sourceSlot": 4,
    "destSlot": 6,
    "exitEdge": "E",
    "exitPosition": 1.0,
    "velocity": { "x": 61.2, "y": 4.4 },
    "heading": 274.11,
    "entityId": -843827577,
    "timestamp": 1785693600149,
    "reroute": {
      "fromSlot": 5,
      "count": 1,
      "proof": "relay_never_forwarded",
      "atMs": 1785693733118
    }
  }
}
```

Note what did **not** change: `timestamp` still names the original journal write, because the
organism left its world once. A re-route is a change of destination, not a new migration.

### 6.7 `MIGRATION_ACK` — sidecar → sidecar, forwarded

**Sent only after the receiving mod's Contract A `MIGRATE_IN_ACK`.** This is the link in
the custody chain that lets the origin sidecar drop its own journal entry (D2, custody
chain step 5). A sidecar **MUST NOT** send `MIGRATION_ACK` when it has merely journaled the
payload.

The one exception is the deduplication path of §6.6 step 1: a `migrationId` that is already
tombstoned was ACKed once before, so re-ACKing it carries the same meaning.

**Pacing does not change this rule, and Risk 9 is the reason to say so out loud.** A paced
delivery can sit in the receiver's journal for minutes of simulated time before the mod sees
it, and the `MIGRATION_ACK` waits that whole time. That silence is not an orphan, and §9.2's
hold clock is what keeps the sender from treating it as one. Moving the ACK earlier — to the
journal write — would fix the silence by breaking the thing the chain exists for: the spawn
is the proof of delivery.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | |
| `sourcePeer` | string | yes | The sender of this ACK — the *receiving* sidecar. |
| `destPeer` | string | yes | The origin sidecar. The relay routes on this. |
| `entityId` | `entityId` | yes | The restored organism's id, echoed from the mod's `MIGRATE_IN_ACK`. |
| `duplicate` | bool | yes | `true` when nothing was spawned because the organism was already there. Treated exactly like a plain ACK. |
| `deliveredAt` | `timestampMs` | yes | Informational. |

On receipt the origin sidecar converts its journal entry into a tombstone, which it keeps
for `exportRetentionSeconds` (3600 s) so a late replay is answered without a second
delivery. **The tombstone keeps the genome hash**, because the archive may ask for that
genome long after the migration completed (§10).

```json
{
  "protocol": "contract-b/3.0",
  "type": "MIGRATION_ACK",
  "messageId": "58d2c0b9-3417-4a6f-9e28-b1d05c7a2f33",
  "sentAt": 1785693600402,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "sourcePeer": "peer-main-slot6",
    "destPeer": "peer-lan-slot4",
    "entityId": -843827577,
    "duplicate": false,
    "deliveredAt": 1785693600399
  }
}
```

### 6.8 `MIGRATION_NACK` — sidecar → sidecar, or relay → sidecar

**Meaning: the receiver did not take durable custody. The organism is still the sender's.**

| Field | Type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | |
| `sourcePeer` | string | yes | The sender of this NACK, or `""` when the relay generated it. |
| `destPeer` | string | yes | The origin sidecar. |
| `code` | string enum | yes | Below. |
| `class` | string enum | yes | `"transient"` or `"permanent"`. An unrecognised `code` is handled as its stated `class` — never switch on `code` without a default branch. |
| `message` | string | yes | Human-readable. Never parsed. |
| `retryAfterMs` | number (int) | no | Present on transient codes. |
| `neverForwarded` | bool | no | **New in M4.** Present exactly on **relay-generated** NACKs. `true` means: *this relay process has forwarded no frame carrying this `migrationId` to any peer, ever, during `relaySessionId`.* `false` means it cannot say that — either it did forward one, or the record does not cover the question. It is a **proof, not a hint** (§5.2, §9.2). |
| `relaySessionId` | `uuid` | no | Present exactly when `neverForwarded` is. The session the statement is scoped to. A sender **MUST** compare it against the session it recorded on the journal entry before treating `neverForwarded: true` as proof. |

| Code | Class | Sent by | Cause |
|---|---|---|---|
| `SLOT_VACANT` | **permanent** | relay | `destSlot` names **no reservation at all** — released, handed to nobody, or never issued. Slot numbers are never reused (§2), so this world never returns and no retry can ever succeed. **Reclassified in M4**: it was `transient` in M3, when a vacant slot and an offline peer were the same answer. |
| `PEER_OFFLINE` | transient | relay | **New in M4.** The reservation exists and its peer has no live connection right now. This is M3's `SLOT_VACANT` case, given its own name so the permanent one can mean what it says. |
| `NOT_FORWARDED` | transient | relay | **New in M4.** The relay declined to hand the frame over for a reason of its own: the destination's outbound queue was full, the write failed before any byte left, or the relay is draining. The slot exists and its peer may well be live. |
| `PEER_UNKNOWN` | transient | relay | `destPeer` is not connected. Applies to `MIGRATION_ACK`, `MIGRATION_NACK` and the genome messages, which route on a peer id rather than a slot. |
| `NOT_A_MEMBER` | permanent | relay | The sender is a read-only subscriber and may not send migrations (§5.1). |
| `OVERLOADED` | transient | sidecar | `inboundQueueMax` reached. Under M4 this is also what a **paced backlog** produces when it stops draining (`contract-a.md` §7.5). |
| `SIM_SIZE_MISMATCH` | transient | sidecar | The two sims disagree about `S`. |
| `MOD_ABSENT` | transient | sidecar | No mod is connected and the queue is full. |
| `INVALID_PAYLOAD` | permanent | sidecar | `bb8-schema` rejected the blob, or it is over `maxPayloadBytes`. |
| `KIND_UNSUPPORTED` | permanent | sidecar | `kind` is not `"bibite"`. |
| `VERSION_UNSUPPORTED` | permanent | sidecar | No `bb8-schema` dialect for `body.version`. |
| `MALFORMED_MESSAGE` | permanent | sidecar | A `data` field failed validation. |
| `SHUTTING_DOWN` | transient | either | The sender is draining. |

**A sidecar MUST NOT send `MIGRATION_NACK` after it has durably journaled the payload.**
This is what makes a sidecar NACK a *definitive* statement that custody never moved, and it
is what makes both the bounce-back and the M4 re-route safe. It is the single most
load-bearing sentence in this document, and the one an implementation is most likely to
violate by adding a NACK to a later failure path. See §9.

**A relay NACK is a statement about one attempt; `neverForwarded` is the statement about the
migration.** A sender that reads `SLOT_VACANT` on its third re-forward learns nothing about
its first: an earlier attempt may have been forwarded to a peer that then died silently. This
is exactly the trap Risk 2 describes, and the boolean is the only thing that gets a sender
out of it (§5.2, §9.2).

**The class change on `SLOT_VACANT` is a wire semantic change and needs care in review.** An
M3 sidecar reads it as transient and retries; an M4 sidecar reads it as permanent and stops
retrying that destination. The major bump (§4) is what stops the two from ever meeting.

**No code in this table is ever caused by the lineage annex.** A missing hash, a gap, an
unknown `gapReason` or an empty `parents` array is never a reason to refuse an organism.
The annex is bookkeeping; the organism is custody.

The relay's answer when the destination slot is reserved, dark, and provably never handed
anything for this migration — the frame that authorizes a re-route:

```json
{
  "protocol": "contract-b/3.0",
  "type": "MIGRATION_NACK",
  "messageId": "b3160fe2-95ad-4c77-8f10-2a4e6c9b0715",
  "sentAt": 1785693733095,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "sourcePeer": "",
    "destPeer": "peer-lan-slot4",
    "code": "PEER_OFFLINE",
    "class": "transient",
    "message": "slot 5 (1,1) is reserved to peer-lan-slot5, dark for 14s; this relay has never forwarded this migration",
    "retryAfterMs": 15000,
    "neverForwarded": true,
    "relaySessionId": "5f0b9c31-77ad-4e26-9a4c-1b83d206ef95"
  }
}
```

The same situation after a relay restart. Everything about the map is identical; the relay
simply cannot speak for the period before it started, so it says `false` and the sender
**holds** instead of re-routing:

```json
{
  "protocol": "contract-b/3.0",
  "type": "MIGRATION_NACK",
  "messageId": "9a41c7e0-3b62-4d85-91fa-6c0e28d3b417",
  "sentAt": 1785693840210,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "sourcePeer": "",
    "destPeer": "peer-lan-slot4",
    "code": "PEER_OFFLINE",
    "class": "transient",
    "message": "slot 5 (1,1) is reserved to peer-lan-slot5, dark for 121s; forwarding record starts at this relay session and cannot cover this migration",
    "retryAfterMs": 15000,
    "neverForwarded": false,
    "relaySessionId": "c8177a34-90bd-4f51-8e02-4d6b915ca7e3"
  }
}
```

### 6.9 `GENOME_REQUEST` — archive or sidecar → sidecar, forwarded

The requester has a genome hash from an annex and no genome behind it. Unchanged from M3
except for the walk order in *who to ask next*.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `requestId` | `uuid` | yes | Correlates the answer. Not an idempotency key — a repeated request is simply answered again. |
| `sourcePeer` | string | yes | The requester. The relay verifies it. |
| `destPeer` | string | yes | The peer being asked. The relay routes on this. |
| `genomeHash` | string | yes | A full `genome-hash.md` value, label included. Never truncated. |
| `context` | object | no | Informational, for the answering peer's logs: `{"migrationId": uuid, "entityId": entityId}` — the annex the hash came from. |

**Who answers.** The peer named in `destPeer`. A requester **SHOULD** ask the envelope's
`sourcePeer` first, because that sidecar hashed and cached the blob (§10). If that answer is
`unknown_hash`, or the peer is offline, the requester MAY ask other live peers **in structural
order** (§6.5), one at a time, honouring the rate limit.

**Answering obligations.** The answering sidecar **MUST** reply with exactly one
`GENOME_RESPONSE` within `genomeRequestTimeoutMs`, from its genome cache, its journal or
its tombstones. It **MUST NOT** block a migration to serve one, and it **MUST NOT** treat a
request it cannot serve as an error.

```json
{
  "protocol": "contract-b/3.0",
  "type": "GENOME_REQUEST",
  "messageId": "6a20f7c8-4b3d-4e51-9017-c8b25d0a4f16",
  "sentAt": 1785693605010,
  "data": {
    "requestId": "d4c19b06-7f52-4a83-b0e1-9c3d7a52f408",
    "sourcePeer": "archive-main",
    "destPeer": "peer-lan-slot4",
    "genomeHash": "bb8-genome/1:sha256:43ea8315112301799622f3ab8906c2882e528502bc35424a209cd67bfdaec207",
    "context": {
      "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
      "entityId": -1180911975
    }
  }
}
```

### 6.10 `GENOME_RESPONSE` — sidecar → requester, forwarded

Also generated **by the relay** when it cannot route the request. Unchanged from M3.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `requestId` | `uuid` | yes | Echoed. |
| `sourcePeer` | string | yes | The answering peer, or `""` when the relay generated it. |
| `destPeer` | string | yes | The requester. The relay routes on this. |
| `genomeHash` | string | yes | Echoed, so a late answer is still attributable. |
| `found` | bool | yes | |
| `body` | object | no | Present exactly when `found` is `true`. |
| `body.version` | string | yes | The game version the blob was serialized by. |
| `body.bb8` | string | yes | The opaque blob, as a JSON string, never nested, never base64. Max `maxPayloadBytes`. |
| `reason` | string enum | no | Present exactly when `found` is `false`: `"unknown_hash"`, `"rate_limited"`, `"peer_offline"` (relay-generated), `"too_large"`, `"shutting_down"`. |
| `retryAfterMs` | number (int) | no | Present on `"rate_limited"`. |

**Requester obligations.** On `found: true` the requester **MUST** recompute the genome hash
of `body.bb8` with `genome-hash.md` and **MUST** discard the answer, log one loud line, and
treat the fetch as failed if the value differs from `genomeHash`. This is content
addressing: a store that trusts the label instead of the bytes is not content-addressed,
and one wrong genome silently poisons every lineage query that touches it.

`"unknown_hash"` is a normal answer, not an error: the source peer may be offline, the parent
may never have migrated, the source may have restarted and lost its cache, or the map may be
busy and the request shed.

```json
{
  "protocol": "contract-b/3.0",
  "type": "GENOME_RESPONSE",
  "messageId": "3f7b1e05-0a94-4d2c-91b7-6d08e5c3a220",
  "sentAt": 1785693605033,
  "data": {
    "requestId": "d4c19b06-7f52-4a83-b0e1-9c3d7a52f408",
    "sourcePeer": "peer-lan-slot4",
    "destPeer": "archive-main",
    "genomeHash": "bb8-genome/1:sha256:43ea8315112301799622f3ab8906c2882e528502bc35424a209cd67bfdaec207",
    "found": true,
    "body": {
      "version": "0.6.3.1",
      "bb8": "{\"transform\":{ ... },\"genes\":{ ... },\"body\":{\"id\":-1180911975, ... },\"brain\":{ ... },\"version\":\"0.6.3.1\"}"
    }
  }
}
```

### 6.11 `PING` / `PONG`

`data` is `{"nonce": "<uuid>"}` on `PING` and the same nonce on `PONG`. Either side may
ping. The relay pings every `relayPingIntervalMs` and closes a peer with `4004` after
`peerTimeoutMs` of complete silence. WebSocket-level ping/pong frames are used as well; the
application-level pair exists so a sidecar can measure the relay path itself.

**A `PING` from a peer MAY carry `stats`** (§6.3.1) beside the nonce — **new in M4**. This is
where a fresh population comes from: a `SECTOR_CLAIM` is sent on change and would leave the
ring view showing the population a world had when it last resized, which is worse than no
number at all.

| Rule | Statement |
|---|---|
| Who | A client with `role: "peer"`. An archive's `PING` carries no stats, and a relay's `PING` never does. |
| Cadence | At most one stats-bearing `PING` per `statsIntervalMs` (5000). A peer with nothing new to say still pings; it simply omits the block. |
| What the relay does | Stores the block against that peer with its own `receivedAt`, and publishes it in the next `PEER_STATUS` (§6.5). It never routes, refuses, schedules or filters on a stat. |
| What it is not | Not liveness — the nonce and the timeout already are. Not backpressure — `OVERLOADED` is. Not a request for anything. |

```json
{
  "protocol": "contract-b/3.0",
  "type": "PING",
  "messageId": "d90c4b71-52ae-4f38-b6c0-1a7e35d20894",
  "sentAt": 1785693731630,
  "data": {
    "nonce": "1b7f0e4a-6c25-4d90-83b1-0e5a7c4d2f16",
    "stats": {
      "population": 244,
      "eggCount": 41,
      "custodyDepth": 3,
      "pacedDepth": 11,
      "heldDepth": 2,
      "bouncedTimeoutTotal": 0,
      "simulatedTime": 120507.75,
      "lastSave": {
        "atMs": 1785693540118,
        "simulatedTime": 119303.50,
        "population": 211,
        "durationMs": 730
      }
    }
  }
}
```

---

## 7. Placement — insertion, growth, release and handover

The relay is the single arbiter of the map (D1, D8, D12). No sidecar chooses its own slot or
its own position, and no sidecar learns the map from anywhere else.

### 7.1 The model

The relay holds a **rectangle** and a set of **reservations**, each binding one slot number
and one position to one `peerId`:

```
map  = { width: 3, height: 2 }

ring = [ {slot: 1, pos: (0,0), peerId: "peer-main-slot1"},
         {slot: 2, pos: (1,0), peerId: "peer-main-slot2"},
         {slot: 3, pos: (2,0), peerId: "peer-main-slot3"},
         {slot: 4, pos: (0,1), peerId: "peer-lan-slot4"},
         {slot: 5, pos: (1,1), peerId: "peer-lan-slot5"},
         {slot: 6, pos: (2,1), peerId: "peer-main-slot6"} ]

east(c, r)  = the first deliverable reservation at (c+1, r), (c+2, r), … modulo width
north(c, r) = the first deliverable reservation at (c, r+1), (c, r+2), … modulo height
```

Both walks skip holes and undeliverable slots, and both stop when they return to the
starting position — a peer never exports to itself (§8).

**Slot numbers are identifiers; positions are geometry.** A slot number is never reused while
its reservation exists, is never renumbered, and is what every journal entry names.
A position is what decides the neighbours, and it moves when the map grows.

**Do not derive a coordinate from an index.** A row-major index over an ordered list is
cheaper to store and renumbers every peer on each insertion, which is exactly the reshuffle
D8 forbids and D12 preserves.

### 7.2 Arbitration rules

On `SECTOR_CLAIM` from a peer, in this order. The first rule that applies wins.

1. **This `peerId` already holds a reservation** → it keeps the slot **and the position**.
   `reason: "reclaimed"` on the first claim of a connection, `"updated"` on a repeat. The
   reservation is keyed on `peerId`, never on a connection, and **never expires**. This is
   the whole of "return needs no insertion": a peer that comes back after two hours or two
   weeks lands where it was, and its neighbours re-pair back to it as a liveness event.
2. **`preferredSlot` names a reservation held by this same `peerId`** → same as rule 1. This
   is the ordinary case after a sidecar restart, replaying `<data-dir>/slot`.
3. **`preferredSlot` names a reservation held by somebody else** → ignore the preference and
   fall through. A preference never evicts anybody.
4. **`preferredPosition` is usable** → grant a new reservation there, with
   `slot = maxSlotEverIssued + 1`. Usable means one of:
   - it names a **hole** inside the current rectangle; or
   - it names a position exactly **one column beyond** the rectangle (`col == width`,
     `row < max(height, 1)`) → `width += 1`, and every other position in the new column
     becomes a hole; or
   - it names a position exactly **one row beyond** the rectangle (`row == height`,
     `col < max(width, 1)`) → `height += 1`, and every other position in the new row becomes
     a hole; or
   - the map is empty and it is `(0,0)` → the map becomes `1×1`.

   Anything else — a taken position, a gap of two columns, a diagonal beyond both axes — is
   ignored and falls through. **A ragged map is never legal**: growth is always by a whole
   column or a whole row, and the rest of it is holes.
5. **`insertAfterSlot` names an existing reservation** → splice on `insertAxis`:
   - `"E"`: insert a new **column** at index `col+1` of that slot. Every reservation with
     `col > that slot's col` has its `col` increased by one. `width += 1`. The newcomer takes
     `(that slot's col + 1, that slot's row)`; the other positions in the new column are
     holes.
   - `"N"`: insert a new **row** at index `row+1` of that slot, shifting every reservation
     above it up by one. `height += 1`. The newcomer takes
     `(that slot's col, that slot's row + 1)`.

   This is the "splice in between two live slots" of D12, on either axis. The predecessor's
   effective neighbour becomes the newcomer, the newcomer's becomes the old successor, **two
   lanes change and no slot number changes.**
6. **Otherwise, auto-place.** In this order:
   - the **first hole in structural order** (`row` ascending, then `col` ascending); else
   - **grow the shorter axis**: when `width ≤ height` add a column at the right end and take
     `(width, 0)`; otherwise add a row at the top and take `(0, height)`; and on an empty map
     take `(0,0)` in a `1×1`.

   `reason: "granted"`.
7. **A claim from a `role: "archive"` client** → `granted: false`,
   `reason: "role_has_no_slot"` (§5.1).
8. **A claim from a peer the relay refused on compatibility grounds** → `granted: false`,
   `reason: "version_incompatible"`, and the refusal appears in `PEER_STATUS`.

**Auto-placement builds the M4 rig's shape with no operator input, and that is the test of
the rule.** Six sidecars starting in order against an empty relay, none of them expressing a
preference:

| Claim | First hole? | Rule 6 does | Slot | Position | Map after |
|---|---|---|---|---|---|
| 1st | — | empty map | 1 | (0,0) | 1×1 |
| 2nd | none | `width(1) ≤ height(1)` → add a column | 2 | (1,0) | 2×1 |
| 3rd | none | `width(2) ≤ height(1)` is false → add a row | 3 | (0,1) | 2×2, hole (1,1) |
| 4th | (1,1) | fill it | 4 | (1,1) | 2×2, full |
| 5th | none | `width(2) ≤ height(2)` → add a column | 5 | (2,0) | 3×2, hole (2,1) |
| 6th | (2,1) | fill it | 6 | (2,1) | 3×2, full |

Six peers with no opinion land in a full **3×2** map, which is the shape the exit test needs
and the smallest honest two-axis map (§2.1). What auto-placement does **not** guarantee is
*which* slot lands where: the exit test describes row 0 holding slots 1–3 and row 1 holding
slots 4–6, and the sequence above gives a different assignment with the same shape. **A rig
that wants a specific layout names it**, and the join kit ships it: each sidecar sends
`preferredPosition` from its own configuration — `(0,0) (1,0) (2,0) (0,1) (1,1) (2,1)` — and
rule 4 grants each one, the first three by growing a column at a time, the fourth by growing
a row, the last two by filling the holes that row created.

"Grow the shorter axis" is therefore the answer for a peer that has no opinion, and it keeps
such a map close to square rather than stretching one axis until route-around has nothing to
route around. It is not a way to build a *particular* map, and it is not meant to be.

**Two peers that claim one position race, and neither fails.** The relay serializes claims and
answers them in arrival order. The second peer's `preferredPosition` is no longer a hole, so
it falls through to rule 6 and lands somewhere. The grant names the position it actually
received, and a lost race is never an error — a peer with no placement has nothing useful to
do, and an operator would have to intervene to give it one.

**Ordering after any change.** The relay **MUST**, in this order:

1. Write `ring.json` (§7.4) and flush it.
2. Answer the `SECTOR_CLAIM` with its `SECTOR_GRANT`.
3. Broadcast `PEER_STATUS` to every client with a **higher `epoch`**.
4. Send a fresh `SECTOR_GRANT` to **every peer whose effective neighbour on either axis
   changed** — which after a splice or a liveness change is usually more than one peer, and
   after a column insertion may be most of them.

A receiver applies the highest `epoch` only. `SECTOR_GRANT` carries no epoch and needs none:
grants to one peer travel that peer's own connection and arrive in order.

**No storm.** Six peers, two axes and a flapping link can generate more registry changes than
anybody can read. The relay **MUST** coalesce: at most one `PEER_STATUS` broadcast per
`statusCoalesceMs` (250 ms), and at most one `SECTOR_GRANT` per peer in that window, always
finishing with a frame that reflects the final state. Coalescing may drop intermediate states;
it **MUST NOT** drop the last one, because every one of these messages is full state and the
last one is the truth.

### 7.3 Placement must not disturb work in flight

Four rules keep every placement safe, and the first three are M3's:

1. **A placement applies to new migrations only.** A journaled outbound entry keeps the
   `destSlot` it recorded. The sidecar **MUST NOT** rewrite a journal entry's destination
   because its effective neighbour changed.
2. **Routing is on the slot** (§5). If the recorded `destSlot` still exists, the frame is
   delivered to whoever holds it, wherever the map has since put them. If it was released,
   the sender gets `SLOT_VACANT` — permanent, and a fact about a world that will not return.
3. **A new `peerId` is a new peer.** A sidecar reinstalled with a fresh `peerId` takes a
   **new** slot, and its old slot stays reserved forever, holding its position. That is why
   `peerId` is persisted outside the journal (§7.4) and why the operator needs the release
   and handover commands (§7.5).
4. **A newcomer's slot number is greater than every number ever issued**, so **no journal
   entry anywhere can name it**. Insertion is therefore provably safe for in-flight work
   rather than argued to be: rule 1 is sufficient because rule 4 makes a stale `destSlot`
   impossible to collide with (D12).

**Positions moving is safe for the same reason.** A splice shifts coordinates, which changes
who is next to whom, which changes future lanes. It changes no address, so it changes nothing
already in a journal. This is the practical payoff of splitting the address from the position
(D13), and it is why a migration frame carries `destSlot` and never a coordinate.

**The one exception to rule 1 is the re-route**, and it carries its own evidence: a sender may
rewrite `destSlot` **only** under a proof of non-delivery (§9.2). Every other entry keeps the
destination it recorded. M3's §7.3 forbade a rewrite outright; M4 narrows the prohibition
rather than lifting it.

### 7.4 What each side persists

| Side | File | Contents | Lost ⇒ |
|---|---|---|---|
| relay | `<data-dir>/ring.json` | The rectangle, the ordered reservation list with **slot, position and `peerId`**, and `maxSlotEverIssued` | The whole map is forgotten; every peer is placed again as a new slot in connect order, and every journaled `destSlot` becomes `SLOT_VACANT`. Durable since M3, and M4 adds the positions and the map. |
| relay | — (memory) | The forwarding record and `relaySessionId` (§5.2) | Nothing is lost that was ever knowledge: a new process cannot prove what an old one forwarded, and it says so with `neverForwarded: false`. **Deliberately not durable.** |
| sidecar | `<data-dir>/peer-id` | One line, the `peerId` | The peer becomes a stranger and takes a second slot, stranding its old one. Generated once, on first start, if absent. |
| sidecar | `<data-dir>/slot` | One line, the granted slot number | Only the `preferredSlot` hint; rule 1 recovers the slot from `peerId` anyway. |
| sidecar | `<data-dir>/position` | One line, `col,row` | Only the `preferredPosition` hint, and only useful to a peer that lost its `peerId` too. Written on every granted `SECTOR_GRANT`. |
| sidecar | journal | Custody, tombstones, the recorded `destSlot`, **the handoff state, the `relaySessionId` of the first hand-off, the accrued hold time and the re-route count** (§9.2) | Organisms. This is the one file whose loss is not recoverable, and D2 accepts that as loss. |

The relay writes `ring.json` **before** it answers a `SECTOR_CLAIM` that created or changed a
reservation — an answered grant that is not on disk can hand the same slot to two peers across
a restart. The sidecar writes `<data-dir>/slot` and `<data-dir>/position` on every granted
`SECTOR_GRANT` and reads them once at startup; an explicit `--slot` or `--position` flag
overrides them, and an unreadable value is treated as absent. Neither file belongs in the
journal: losing them costs a placement mix-up, never an organism.

**The journal's new fields are load-bearing and must survive a restart.** §9.2 depends on
every one of them: the handoff state says whether custody may have moved, the `relaySessionId`
scopes the proof, the accrued hold time is what a bounded hold counts, and the re-route count
is what bounds it. A sidecar that reconstructs any of them from memory at startup has lost
the safety property, not just the bookkeeping.

### 7.5 Operator commands — release, handover, and the custody rules around them

The reservation never expires, so the map needs operator escape hatches. There are three, and
**none of them is a wire message**: an operator command is a rare, deliberate, physical act on
the machine that owns the data, and giving it a network surface in a milestone with one shared
token is a poor trade (§3.1). M5 brings authentication, and §13 item 5 records that release
and handover are the first commands that will want an authenticated admin path.

| Command | Where | What it does |
|---|---|---|
| `multiverse-relay --release-slot <n>` | relay, at startup | Removes slot `n` from `ring.json` and leaves its position a **hole**. Surviving slots keep their numbers, their positions and their relative order. The number is not reused: `maxSlotEverIssued` never decreases. |
| `multiverse-relay --handover-slot <n> <newPeerId>` | relay, at startup | **New in M4** (D15). Rebinds the reservation — slot number **and** position — to a different `peerId`. The map does not change shape and no lane moves. |
| `multiverse-sidecar --release-inflight <migrationId> bounce\|drop` | the sidecar that holds the journal | **New in M4** (D2, D12). Releases one **held** entry by hand, before the hold timeout expires (§9.3). |

**A release or a handover never moves a journal** (D12, Question 3). This is the rule that
everything else here follows from:

- **The peer that holds a journal holds the organisms in it.** No other peer claims them, and
  no protocol transfers them. A transfer protocol is a duplication mechanism with a friendly
  name.
- **A released or replaced peer keeps two obligations.** It delivers its inbound entries to
  its own mod, even outside the map. It re-routes or releases its outbound entries when it
  rejoins, under §9.2's rules, exactly like any other returning peer.
- **The new occupant of a handed-over slot inherits nothing.** It starts with an empty
  journal and a different world. It is not told about the old peer's entries, because it
  cannot do anything correct with them.
- **In-flight work addressed to that slot arrives at the new occupant**, because routing is
  on the slot (§5, §7.3 rule 2). That is deliberate: a handover rebinds *the position in the
  map*, and an organism travelling to that position arrives at whoever is there. An operator
  who does not want that outcome wants `--release-slot`, which makes the address permanently
  vacant instead.
- **The relay refuses a handover while the old peer is live**, and says so. A live peer with
  its slot taken out from under it would keep claiming, keep being refused, and keep a world
  running with nowhere to export.

**Print the consequences before the act.** Both relay commands **MUST** print, and require a
confirmation for, the full ring-side consequence: the slot, its position, its `peerId`, how
long it has been dark, which peers' effective lanes change, and which positions become holes.

**What the relay cannot print, and what does instead.** `m4_considerations.md` Question 3 asks
both commands to list the held entries that name the slot. The relay **cannot**: journals live
on six other machines and D2 keeps custody local — a relay that could enumerate them would be
a relay that reads journals. The division is therefore:

| Question | Answered by |
|---|---|
| What happens to the map? | The relay command's own pre-act report. |
| Who is holding organisms addressed to this slot? | `PEER_STATUS.slots[].stats.heldDepth` (§6.3.1) — visible on the status page and in `ringstat` for every peer at once. |
| **Which** entries, and what are they? | `multiverse-sidecar --list-inflight [--dest-slot <n>]`, on the machine that holds the journal. It prints each entry's `migrationId`, `entityId`, `destSlot`, handoff state, accrued hold and deadline. |

The operator therefore makes one decision with the facts in view; the facts simply come from
two places, because custody does.

---

## 8. From `PEER_STATUS` to `EDGE_STATUS` — the per-axis walk

A sidecar's only job with the map is to decide whether **each of its own export edges** is
open, and to tell its mod (`contract-a.md` §5.4). The mapping is exact and belongs here,
because it is the sidecar's decision. It replaces M3's single-edge table, and the M3 table is
its height-1 case.

**Deliverability, not liveness, selects the target** (D12). M3 evaluated a list of conditions
and **closed the edge** on the first failure. M4 evaluates the same conditions per candidate
slot and uses them to **filter**:

```
deliverable(slot) ⇔ slot is reserved            (a hole is not deliverable)
                  ∧ live                        (a relay connection exists)
                  ∧ modConnected                (something can spawn an organism)
                  ∧ gameVersion compatible      (with mine)
                  ∧ simulationSize equal to mine (contract-a.md §13, A10)
                  ∧ slot ≠ me                   (a peer never exports to itself)
```

The walk, per export edge, over the positions of `PEER_STATUS`:

```
effective(E) = first deliverable slot at (col+1, row), (col+2, row), … mod width
effective(N) = first deliverable slot at (col, row+1), (col, row+2), … mod height
```

Each walk visits every other position on its axis exactly once and then stops. The relay
performs it and publishes the result in `SECTOR_GRANT.neighbours` (§6.4); a sidecar uses the
grant, and any client may reproduce the walk from `PEER_STATUS` for display (§6.5).

| Condition, evaluated in order, **per export edge** | `EDGE_STATUS` entry for that edge |
|---|---|
| No relay connection | `open: false`, `reason: "peer_unreachable"` |
| No slot granted | `open: false`, `reason: "no_peer"` |
| The grant carries no `neighbours` key for this edge | `open: false`, `reason: "no_peer"` — **or the shared skip reason**, see below |
| Operator closed this edge locally | `open: false`, `reason: "admin_closed"` |
| Otherwise | `open: true`, `reason: "peer_live"`, `peerSimulationSize` = the effective neighbour's `S` |

**Which reason a closed edge reports.** A closed edge under route-around means *no slot on
that axis was deliverable*, which is an aggregate of individual skip reasons. Contract A's
`reason` enum is unchanged, so the aggregate has to map into it, and the mapping is
deterministic:

| The axis's `skipped` list | `EDGE_STATUS.reason` for that edge |
|---|---|
| Empty — there was no other position on the axis at all | `no_peer` |
| Every entry shares the reason `peer_mod_absent` | `peer_mod_absent` |
| Every entry shares the reason `peer_incompatible` | `peer_incompatible` |
| Every entry shares the reason `sim_size_mismatch` | `sim_size_mismatch` |
| Every entry shares the reason `hole` or `peer_offline` | `no_peer` |
| Entries disagree | `no_peer` |

A two-world column whose other world was resized therefore tells the mod
`sim_size_mismatch` rather than a shrug, and a three-world row that lost two peers for two
different reasons says `no_peer` and leaves the detail to the status page, which has the
`skipped` list itself. Three Contract A reasons never come out of this mapping:
`peer_unreachable` (the sidecar's own relay connection is down, decided before the walk),
`admin_closed` (local), and `peer_overloaded` — which no M3 mapping emitted either, because
overload is a runtime refusal on a delivery (`OVERLOADED`, §6.8) and never a registry fact the
relay holds. All three stay in the enum: removing an enum value is a major bump for nothing.

**An export edge closes for one reason only: no slot on that axis is deliverable.** Every
other M3 close reason has been demoted to a **skip** reason, and each keeps its name in
`SECTOR_GRANT.neighbours.<edge>.skipped[].reason` (§6.4). This is the whole of D12 on one
line, and it is what T1's two-hour outage cost: under M3 slot 1 closed its export edge and its
organisms walked out of the playable square instead of migrating.

**The ripple keeps its direction.** A slot that goes dark is announced *backwards* along each
lane that pointed at it:

- every peer whose effective neighbour **was** that slot re-targets and **keeps its export
  edge open** — it gets a fresh `SECTOR_GRANT` and, if the edge state changed at all, a fresh
  `EDGE_STATUS` with a `skipped` entry behind it;
- the dark slot's own **east and north** neighbours are unaffected and are told nothing; they
  simply receive nothing from it;
- the dark slot's reservation stays in the map, `live: false`, with `darkSinceMs` set.

A sidecar that loses its **mod** publishes `modConnected: false` on its next `SECTOR_CLAIM`,
and the relay's `PEER_STATUS` carries it to everyone pointed at it, which re-pairs their lanes
around it. A dead sim must not keep receiving organisms (`contract-a.md` §8, step 2) — and
under M4 it must also not stop the current.

**A bypass is a warning, not a normal state** (Risk 5). A healed map keeps working, which is
the point and also the failure mode: an operator sees a healthy current and misses a world
that went dark a day ago. `darkSinceMs` (§6.5) and the `skipped` lists (§6.4) are what the
status page and `ringstat` render as "slot 5 bypassed since 04:12, receiving nothing".

---

## 9. Custody — the chain, the handoff state, the re-route and the bounded hold

Contract A owns steps 1, 2, 4 and 5 of `system_decomposition.md`'s custody chain. Contract B
owns the middle. **M4 adds one bounded exception to at-most-once and nothing else**, and D2's
amended text is the authority for it: an organism forwarded to a slot that then went dark,
with no proof of non-delivery, is held for a configurable timeout — 24 hours by default — and
then bounces home by itself. The owner signed that off on 2026-08-05 and accepts the residual
duplication risk it names (§9.3).

### 9.1 The chain

```
mod A            sidecar A                relay             sidecar B            mod B
  │ MIGRATE_OUT      │                      │                   │                  │
  ├─────────────────►│ hash + journal+fsync │                   │                  │
  │ MIGRATE_OUT_ACK  │  (handoff: pending)  │                   │                  │
  │◄─────────────────┤                      │                   │                  │
  │  (destroys)      │ MIGRATION_PAYLOAD    │                   │                  │
  │                  ├─────────────────────►├──────────────────►│ journal + fsync  │
  │                  │  (handoff: sent)     │  (records the id) │  custody moves   │
  │                  │                      ├╌╌► archive (copy) │ MIGRATE_IN       │
  │                  │                      │                   ├──── paced ──────►│
  │                  │                      │                   │ MIGRATE_IN_ACK   │
  │                  │                      │                   │◄─────────────────┤
  │                  │  MIGRATION_ACK       │                   │                  │
  │                  │◄─────────────────────┤◄──────────────────┤ (tombstone)      │
  │                  │ (tombstone)          ├╌╌► archive (copy) │                  │
```

Every hop deduplicates on `migrationId`, so any frame in this diagram can be replayed safely.
The dotted copies are best-effort and hold nothing up (§5.1). The `MIGRATE_IN` arrow is
**paced** (`contract-a.md` §7.5): custody is taken at the speed of the wire and released at
the speed of the receiving world.

### 9.2 The handoff state, and when a journaled hop may be re-routed

**The problem, stated exactly.** A journaled hop names the slot it recorded. That slot goes
dark. Re-routing it east is what keeps the current flowing (D12) — and it is also how one
organism becomes two, if the far sidecar had already taken custody, died before its
acknowledgement, and replays its own journal when it returns. From the sender, that case and
"the frame never arrived" look **identical**: both are silence.

**The answer is evidence.** A sender re-routes only under a statement that no custody was
taken, and holds otherwise. Silence is never such a statement.

Every outbound journal entry therefore carries a **handoff state**, durable (§7.4):

| State | Meaning | Custody may have moved? |
|---|---|---|
| `pending` | Journaled. Never written to a live relay connection. | **No** |
| `sent` | Written to a live relay connection, no terminal answer yet. Carries the `relaySessionId` in force at that first write. | **Yes — unknowably** |
| `held` | `sent`, and the destination is observed dark. The hold clock accrues here and nowhere else (§9.3). | **Yes — unknowably** |
| `refused` | A statement arrived that proves no custody moved. | **No** |
| `done` | `MIGRATION_ACK` received. Becomes a tombstone. | It moved, and completed. |

Transitions:

```
journal write ─────────────────► pending
pending ── written to relay ───► sent          (record relaySessionId)
sent ── MIGRATION_ACK ─────────► done
sent ── proof of no custody ───► refused
sent ── destination dark ──────► held
held ── destination live ──────► sent          (clock stops, accrual is kept)
held ── proof of no custody ───► refused
held ── accrual ≥ holdTimeout ─► bounced home  (§9.3)
refused / pending ── lane exists ─► re-route, then pending against the new destSlot
refused / pending ── no lane, bounceTimeoutMs ─► bounced home
```

**A bounce is a terminal action, not a sixth state.** The entry leaves the outbound journal,
becomes an inbound delivery into this peer's own mod (§9.4), and leaves a tombstone behind.
The tombstone matters: a `MIGRATION_ACK` that arrives *after* a timeout bounce is absorbed
rather than re-processed — and **logged at error level**, because it is the accepted
duplication case of §9.3 announcing itself. That late ACK is the only signal the map will ever
give that a bounded hold produced two organisms, and an implementation that swallows an ACK
for an unknown `migrationId` silently throws it away.

**What counts as proof of no custody**, and nothing else does:

| Evidence | Why it proves it |
|---|---|
| The entry is `pending` | The frame was never handed to anybody. The sender's own record is sufficient. |
| A **sidecar-generated** `MIGRATION_NACK` with a peer-local code — `OVERLOADED`, `SIM_SIZE_MISMATCH`, `MOD_ABSENT` | §6.8 forbids a sidecar to NACK after it has durably journaled, so a NACK *is* the receiver stating it took no custody. |
| A **relay-generated** `MIGRATION_NACK` with `neverForwarded: true` **and** a `relaySessionId` equal to the one recorded on the entry | The relay has forwarded no frame with this `migrationId` to anyone during a session that covers the entry's whole life (§5.2). |

| **Not** evidence | Why not |
|---|---|
| Silence, of any duration | The exact shape of the dangerous case. |
| `neverForwarded: true` with a **different** `relaySessionId` | The relay restarted; it cannot speak for what the previous process forwarded. |
| A relay NACK carrying `neverForwarded: false` | The relay either did forward this migration once, or cannot say. Both mean the same thing to a sender: no proof. |
| A relay NACK carrying **no** `neverForwarded` field at all | A conforming relay always sets it on a NACK it generated (§6.8), so a missing field is a defect or a frame that came from somewhere else. Treat a missing proof as no proof, and log it. |
| The `code` alone, on any single attempt | Every relay `code` describes **that attempt**. Attempt 1 may well have been forwarded, and only the boolean speaks about the whole migration. |
| `SLOT_VACANT` on its own | It proves the destination will never return, which is a reason to stop retrying — not a statement that nothing was ever delivered there. |

**The rule, as the design states it** (`m4_considerations.md`, Question 2):

| Journal entry state | Evidence | Action |
|---|---|---|
| Never forwarded | `pending`, or a relay `SLOT_VACANT` / `PEER_OFFLINE` / `NOT_FORWARDED` carrying a matched `neverForwarded: true` | **Re-route** along the same axis to the current effective neighbour. Keep the `migrationId`. Rewrite `destSlot`. |
| Refused for a peer-local reason | `MIGRATION_NACK` with `OVERLOADED`, `SIM_SIZE_MISMATCH` or `MOD_ABSENT` | **Re-route.** The receiver stated it took no custody, and another slot accepts the same organism. |
| Refused for a payload reason | `MIGRATION_NACK` with `INVALID_PAYLOAD`, `KIND_UNSUPPORTED`, `VERSION_UNSUPPORTED` or `MALFORMED_MESSAGE` | **Bounce home.** Every slot refuses this organism, so the map is not the answer. |
| Forwarded, then silence | none | **Hold, then bounce** (§9.3). Retry the recorded `destSlot` on the retry cadence; the retry is idempotent because the destination deduplicates. |

**Re-route mechanics.**

| Rule | Statement |
|---|---|
| The key never changes | The `migrationId` is preserved. A re-routed frame that races a late delivery is deduplicated at whichever peer sees it second (§6.6 step 1) — this is what makes a re-route safe even when the proof was, against all the rules above, wrong. |
| The axis never changes | An entry that left through `E` re-routes to the current `effective(E)`, and one that left through `N` to `effective(N)`. The organism left through one door and it keeps going that way; `exitEdge`, `exitPosition`, `velocity` and `heading` are untouched. |
| Only `destSlot` is rewritten | Plus the informational `reroute` block (§6.6). `timestamp`, the annex and the body are the same bytes. |
| It needs a lane | With no effective neighbour on that axis there is nowhere to re-route to. The entry waits, and bounces home after `bounceTimeoutMs` — M3's rule, unchanged, and now reached only when route-around has no answer either. |
| It is bounded | `reroute.count` increments on each rewrite and **MUST NOT** exceed `maxReroutes` (4). Beyond that the entry bounces home. An organism circling a broken axis is a symptom, not a delivery strategy. |
| It is reported | Each re-route is a log line and a metric, and the frame carries its own `reroute.proof`. Part 2 of the exit test requires every in-flight entry to state which answer it took and why. |

**Why `pending` is worth a state of its own.** Without it, an entry the sender never even
managed to send would be indistinguishable from one that was sent into silence, and the safe
answer for the ambiguous case — hold for a day — would be applied to the case that is not
ambiguous at all. Most re-routes in a real outage are `pending` entries: the west neighbour
noticed the gap before it wrote anything.

### 9.3 The bounded hold, and the clock that runs only while the destination is dark

An entry in `held` is a forwarded organism with no proof of non-delivery and a dark
destination. It waits. **The wait is bounded**, and then the organism comes home.

| Rule | Statement |
|---|---|
| The clock | `holdTimeoutMs`, default **86 400 000 ms (24 hours)**, of **accrued** hold time. |
| **When it runs** | Only while **all three** hold: the entry is in `held`; the sender has a **live relay connection**; and the destination slot is, in the sender's latest `PEER_STATUS`, **not live** (`live: false`) or **absent from the map** entirely. |
| **When it stops** | The moment any of those stops being true. The destination coming back stops it. The **sender** losing its own relay connection stops it. A relay restart stops it until the sender reconnects and sees the map again. |
| What it never counts | Time while the destination is **live**. A live peer with a deep paced backlog is **slow, not orphaned** (Risk 9), and pacing can make a live peer silent for a long simulated while. Counting that time would bounce an organism that was about to be spawned — and that is a duplication, not a delay. |
| What it never counts, either | Time while the **sender** is blind. A sender whose own machine slept for a night must not wake with an expired clock: it never observed the destination dark, it only failed to observe anything. T1 was exactly that machine. |
| Where it lives | In the journal entry, as **accrued milliseconds**, flushed at least every `holdAccrualFlushMs` (60 000) and on clean shutdown. On restart the sidecar **resumes the accrual** and **MUST NOT** start a fresh timer. |
| Why accrual and not a deadline | `m4_considerations.md` says "the deadline lives in the journal entry" and means "the timer survives a restart and never resets". A wall-clock deadline cannot express a clock that stops, so the entry carries the accrual instead. It satisfies the requirement more strictly: a restart cannot lose time already served, and cannot invent time that was never served. |
| A crash | Loses at most one flush interval of accrual, in the safe direction: the entry waits slightly longer. |
| At the timeout | The sidecar **bounces the organism home by itself** — an ordinary Contract A `MIGRATE_IN` with `bounceBack: true` into its own mod, on the edge it left from (§9.4) — records the reason on the entry, increments `stats.bouncedTimeoutTotal` (§6.3.1), and logs one loud line. |
| Reporting | **An automatic bounce is a fact the operator reads, not a silent repair.** The status page and `ringstat` both name every bounce a timeout caused, with the migration, the entity, the destination slot and the accrued hold. |

**The accepted duplication case, named so nobody is surprised by it.** One path produces two
organisms: the far sidecar took custody, died before its acknowledgement left, the sender's
hold expired and bounced the organism home, and the far sidecar later returned and replayed
its own journal to its own mod. It needs an invisible delivery **and** a return after the
timeout. The owner weighed it against the alternative and chose it: an unbounded hold strands
an organism in a file nobody reads, and a stranded organism is invisible, whereas a
duplication is at least visible in a whole-map entity-ID count. **At-most-once carries this
one exception and no other. Do not widen it.**

**And it is detectable after the fact.** When the far peer returns, delivers the organism it
had custody of and sends its `MIGRATION_ACK`, that ACK arrives at a sender whose entry is
already a bounce tombstone. The sidecar **MUST** log it at error level, name the
`migrationId` and the `entityId`, and count it in a `duplicateSuspected` metric. It is the
only automatic signal that this exception fired, and the exit test's whole-map entity-ID count
is the manual one.

**The manual escape hatch.** `multiverse-sidecar --release-inflight <migrationId>
bounce|drop` releases one held entry by hand before the timeout expires. It is the custody
twin of `--release-slot`, it runs on the machine that holds the journal, and it
**MUST** print the duplication risk in its own output before acting. `--list-inflight`
(§7.5) is how the operator finds the entry.

**A destination whose slot is vacant will run the whole clock, and that is deliberate.** A
released slot never returns (§2), so holding cannot help — but the hold is not about hope,
it is about the possibility that custody already moved to a peer that is gone. One rule, one
timeout, one place to reason about it; the operator who does not want to wait has
`--release-inflight`.

### 9.4 Bounce-back

**Custody chain step 6 says a remote NACK or a timeout re-injects the organism into the
origin sim, and the timeout half is where duplication lives.** The rule is narrowed —
unchanged from M2 and M3 in its first three rows, with M4's bounded hold as the fourth
(`contract-a.md` §13, A6):

| Situation | Origin sidecar's action |
|---|---|
| `MIGRATION_NACK` received, any code | **Bounce**, or re-route first where §9.2's table says so. A NACK is only ever sent before durable custody (§6.8), so it proves the organism is not at the destination. |
| The forward never reached a live peer — the relay link is down, or `destSlot` is vacant — for longer than `bounceTimeoutMs`, **and route-around has no lane to offer** | **Bounce.** The frame was never handed to anyone. |
| The forward reached a live peer and no answer came back, and the destination is **live** | **Hold and re-forward.** Never bounce. The destination deduplicates, so a re-forward is free, and holding converts a possible duplication into a bounded delay. |
| The forward reached a live peer and no answer came back, and the destination is **dark** | **Hold with the clock of §9.3**, then bounce at the timeout. |

A bounce re-delivers the organism to the origin's own mod as a Contract A `MIGRATE_IN` with
`bounceBack: true`, **on the edge it left from** — the origin's own `exitEdge`, `"E"` or
`"N"`, not a passive entry edge. The organism therefore lands inside its own capture band,
moving outward, and only the entry-immunity window stops it re-exporting on the next tick;
`contract-a.md` §14 A11 and §15 A19 make that window REQUIRED for exactly this reason.

**The mirror-image rule protects the destination.** When the destination mod answers
`MIGRATE_IN_NACK` with a **permanent** code after the payload was already journaled, the
destination sidecar **holds** the entry in its journal for an operator and logs it loudly. It
does not return the organism over Contract B, because there is no two-phase return in M4's
message list and a lost return frame would lose the organism outright (`contract-a.md` §5.9,
§13 A7; §13 item 2 below).

**A returning peer reconciles its own journal**, and it is the same four rules the map uses
for everyone else (`m4_considerations.md`, Question 4):

1. **Reclaim first.** It claims its slot with its persisted `peerId` and gets the same slot
   and the same position (§7.2 rule 1). Its neighbours' lanes re-pair back to it on the next
   liveness change.
2. **Replay the inbound entries.** Every un-acknowledged `MIGRATE_IN`, in journal order,
   through the rate limit (`contract-a.md` §7.5).
3. **Re-evaluate the outbound entries under §9.2.** A `pending` entry re-routes; a `sent` or
   `held` entry retries its recorded destination and keeps its accrued hold.
4. **Reassert custody against the mod.** A new `sessionId` triggers `contract-a.md` §7.4.

**No backlog waits at a returning peer.** Its neighbours re-routed only the entries they had
never forwarded, so nothing accumulated behind the gap in *their* journals. What did
accumulate is the peer's own inbound queue from before it went dark, and that is exactly the
dam the delivery rate limit paces (`contract-a.md` §7.5, §15 A20).

---

## 10. The archive: genomes, gaps, rate limits and the status page

The archive records every envelope it is copied (§5.1), stores genomes by hash, and builds
the lineage graph from the annexes. Everything it does is off the migration path.

**The genome cache, sidecar side.** A sidecar caches a genome blob under its
`genome-hash.md` value from three sources:

| Source | When |
|---|---|
| A parent blob on Contract A `MIGRATE_OUT` | At export, before the blob is stripped (`contract-a.md` §14, A12) |
| The migrant's own `body.bb8` | On export, and again on inbound journal write (§6.6 step 6) |
| Its own tombstones | A completed migration keeps its genome hash, so a hash can be served after the journal entry is gone |

The cache is **on disk**, under `<data-dir>/genomes/`, sharded by the first two hex
characters of the digest. It survives a restart. Entries expire after
`genomeCacheRetentionDays` (30) with a least-recently-served policy, bounded by
`genomeCacheMaxBytes` (2 GiB).

**Fetch behaviour, archive side.**

| Rule | Statement |
|---|---|
| Never block | A migration record is written when the envelope arrives. A missing genome is a **gap** on that record, never a reason to delay or refuse a record. |
| Ask the source first | The envelope's `sourcePeer` hashed the blob and cached it, so it is the peer most likely to have it. |
| Then ask around | On `unknown_hash`, try other live peers in structural order, one at a time. |
| Rate limit | At most `genomeRequestsPerMinute` (30) requests from one requester to one peer. The answering sidecar enforces the same limit and answers `rate_limited` above it. |
| Retry schedule | 1 minute, 5 minutes, 30 minutes, 6 hours, then daily. Reset the ladder when the map's membership changes — a peer that just came back may hold what nobody had. |
| Keep the hash forever | A hash with no genome is a permanent, useful record: it is still a lineage-graph node, and a fetch that failed for a year can succeed tomorrow. |
| Verify | Recompute the hash of every fetched blob (§6.10). |
| Report | The gap report lists every hash no peer has served, with its first-seen time and attempt count. It is the archive's honest statement of what it does not have. |

**The known limit, restated because it is load-bearing** (D11): a database built from
migrations holds migrants and their ancestors, never the resident population of a peer.
Census uploads would close the gap and are not M4.

### 10.1 What the status page may claim, and what it must call unknown

**New in M4 (D15).** The archive serves a live HTML status page and backs the `ringstat`
terminal command. The page's rendering is not a wire contract and this document does not
specify it. **Its inputs are**, and three rules keep them honest (Risk 4):

| Rule | Statement |
|---|---|
| One source, no polling | Everything the page shows about the map comes from the `PEER_STATUS` broadcasts the archive already receives as a subscriber (§5.1, §6.5), plus the envelope copies it already records. The page **MUST NOT** connect to a sidecar, read another component's files, or ask the relay for anything. Nothing on the migration path may ever wait for a reader. |
| Derived, and marked as derived | The effective lanes and the bypasses are **recomputed** by the walk of §8 from `PEER_STATUS`. They are the same computation the relay performs, on the same inputs, and they are for display: the relay's `SECTOR_GRANT` remains the authority for a peer's actual routing. |
| Unknown is a value | A field absent from `stats`, a slot with no `stats` at all, a `statsAsOfMs` older than `statsStaleMs` (30 000) — every one of them renders as **unknown**, never as zero and never as the last value seen without its age. A slot that reports nothing is unknown, not empty. **An honest gap beats a confident zero.** |

The exit test asks the page for the map and its holes, each slot's liveness and population,
each effective lane, each bypass with the time it went dark, each sidecar's custody depth,
each bounce a hold timeout caused, the last save of each world, and the paced journal depth.
Every one of those is a field of §6.5 or §6.3.1, or is derived from them by §8 — which is why
those two sections carry fields that no routing decision reads.

---

## 11. Deviations, and why

1. **`SECTOR_CLAIM` and `SECTOR_GRANT` keep their names, for the third milestone running.**
   The unit they carry is a slot with a position, not a sector. `system_decomposition.md`'s
   ratified Contract B list still names both messages, and renaming a ratified message to fix
   a noun would put this document and the decomposition out of step for no wire benefit. The
   **fields** are renamed wherever the noun was wrong — `slot`, `position`, `slotCount`,
   `neighbours`, `sourceSlot`, `destSlot` — so nothing inside a frame says "sector" or
   "ring".
2. **JSON, not Protobuf.** Contract B's format was left open ("Protobuf likely"). M4 keeps
   the JSON envelope of Contract A so one codec, one test harness and one set of eyes cover
   both wires. The `bb8` body is still an opaque string, so the D4 boundary is unchanged and
   a later Protobuf framing changes no message shape.
3. **`entityId` and `heading` are carried explicitly** (§6.6), even though Contract C's
   field list has neither. Contract A's `MIGRATE_IN` requires both. **Which value wins is
   not the same for the two fields, and the asymmetry is deliberate** — carried unchanged
   from `contract-b-m2.md` amendments B1 and B2 through M3:

   | Field | Authority | Why |
   |---|---|---|
   | `entityId` | The **blob**, when `bb8-schema` can read one. A wire value that disagrees is overridden and logged as a warning. | The blob is what `LoadBibiteOrEggFromData` restores, so the blob's id is the id that will actually exist. The mod's durable dedup key (`contract-a.md` §7.3) has to match it. |
   | `heading` | The **wire**. The blob's `$.rb2d.r` fills in only when the wire field is absent or zero. | The receiving mod rewrites `$.transform.rotation` and `$.rb2d.r` from the `MIGRATE_IN` heading before it restores (`contract-a.md` §5.7 step 3), so the wire value is authoritative by construction. |

   **The shape of `$.body.id` is not fixed by Contract A.** Its §5.3 example elides the
   `BibiteID` wrapper — `"body":{"id":-843827577, ... }` — while the game's own type is
   `BibiteID` with an inner `id`. `bb8-schema` **MUST** accept both: `{"body":{"id":N}}` and
   `{"body":{"id":{"id":N}}}`. Extraction is best-effort throughout.
4. **`HANDSHAKE_ACK`, `SECTOR_GRANT` and `PEER_STATUS` are not in the ratified message
   list.** It names only the requests. A relay-arbitrated map needs a reply and a liveness
   broadcast; these three are the smallest set that provides them. Carried from M2.
5. **The relay's map memory is durable** (§7.4), and its **forwarding record is not**. The
   first is a correctness requirement, because a reservation that never expires is worthless
   in RAM. The second is the opposite: the record is a claim about what *this process* did,
   and persisting it across a restart would let a new process assert knowledge it does not
   have (§5.2).
6. **The relay gained two rules that are not frame forwarding**: the subscriber fan-out
   (§5.1, from M3) and the effective-neighbour walk with its forwarding record (§5.2, §8, new
   in M4). D1 keeps the relay dumb, and it stays dumb in the sense that matters — it never
   parses a body, never validates a payload, never indexes a genome and never stores an
   organism. The walk reads the registry it already keeps; the record is a set of ids it
   already routed.
7. **`entryEdge` is still not on the wire.** The receiving sidecar derives it from `exitEdge`
   by the opposite-edge function (§6.6), and for a bounce-back from its own peer's exit edge.
   Both are facts the receiver already knows about itself, and sending it would let a sender
   dictate a receiver's geometry.
8. **The non-delivery proof is a field on an existing message, not a new message pair**
   (§5.2, §6.8). A `FORWARD_QUERY` / `FORWARD_STATUS` pair was the obvious alternative and it
   is worse: the sender's re-forward **is** the query, so a separate ask would be a second
   round trip carrying no new information, and a proof that can be requested independently of
   an attempt invites an implementation that asks once and caches the answer — which is
   exactly how a stale proof becomes a duplicated organism.
9. **Population and the operational stats ride on `PING`, not on `SECTOR_CLAIM` alone**
   (§6.3.1, §6.11). The work order puts `population` on the claim, and it is there. A claim is
   sent **on change**, though, so a ring view fed only by claims shows the population each
   world had when it last resized. The periodic frame that already exists is `PING`, so the
   stats block rides along with it and the relay copies the latest into `PEER_STATUS`. No new
   message, no new cadence, and the relay still interprets nothing.
10. **The status page reads the ring view, not the sidecars' metric files** (§10.1). Durable
    per-sidecar metrics still exist and are WP3's deliverable, but two of the six worlds run
    on a machine the archive cannot read a file from, and D9 keeps it that way. Anything the
    page must show for **every** world therefore travels the wire as a stat.
11. **"Fall back to the tail" became "fill the first hole, then grow the shorter axis"**
    (§7.2 rule 6). `m4_considerations.md` Question 5 says the relay honours a position that
    exists and falls back to the tail. A rectangle has no tail: the M3 ring's tail was the end
    of one ordered list, and the M4 map has one list per row and one per column. Rule 6 is the
    generalization that keeps the intent — a peer with no opinion gets *some* placement,
    deterministically, without an operator — and it reproduces the exit test's 3×2 shape from
    six opinion-free claims. On a one-row map it is exactly the old tail rule: the first hole,
    or a new column at the right-hand end.
12. **The relay's pre-act report cannot list held entries, and the split is stated instead**
    (§7.5). Question 3 asks `--release-slot` and `--handover-slot` to list the held entries
    that name the slot. Journals live on the peers' own machines and D2 keeps custody local, so
    a relay that could enumerate them would be a relay that reads journals — which is a bigger
    change to D1 than anything else in this milestone. The relay prints the map-side
    consequences; `stats.heldDepth` (§6.3.1) shows which peers are holding anything at all on
    the status page; and `multiverse-sidecar --list-inflight --dest-slot <n>` prints the
    entries themselves on the machine that owns them. The operator still decides with the facts
    in view.

---

## 12. Tunables and defaults

| Name | Default | Owner | Meaning |
|---|---|---|---|
| `relayPort` | `8790` | both | The relay's listen port. Opened in the Windows Firewall on the relay's machine. |
| `relayPingIntervalMs` | `5000` | relay | Application-level `PING` cadence. |
| `peerTimeoutMs` | `15000` | relay | Silence before a peer is dropped and its slot goes `live: false`. |
| `relayBackoffMinMs` | `1000` | client | Reconnect floor. |
| `relayBackoffMaxMs` | `30000` | client | Reconnect ceiling. |
| `stableSessionMs` | `5000` | client | How long a connection must live before the backoff ladder resets (`contract-a.md` §13, A8). |
| `authFailuresBeforeCeiling` | `5` | client | Consecutive HTTP 401s before the backoff is pinned at the ceiling (§3.1). |
| `statusCoalesceMs` | `250` | relay | Minimum spacing between `PEER_STATUS` broadcasts, and between grants to one peer. The last frame of a burst is always sent (§7.2). |
| `statsIntervalMs` | `5000` | sidecar | Minimum spacing between stats-bearing `PING`s (§6.11). |
| `statsStaleMs` | `30000` | archive | Age at which a `stats` block renders as unknown rather than as state (§10.1). |
| `forwardRetryMs` | `5000` | sidecar | Re-forward cadence for a journaled outbound entry with no answer. |
| `bounceTimeoutMs` | `20000` | sidecar | How long an outbound entry that never reached a live peer, **and has no lane to re-route to**, waits before it is bounced home (§9.2, §9.4). |
| `migrationAckTimeoutMs` | `30000` | sidecar | Informational deadline for `MIGRATION_ACK`; expiry re-forwards, it never bounces (§9.4). |
| `holdTimeoutMs` | `86400000` | sidecar | **New in M4.** Accrued dark time before a held entry bounces home by itself — 24 hours (D2, signed off 2026-08-05). The clock runs only while the destination is dark and the sender can see it (§9.3). |
| `holdAccrualFlushMs` | `60000` | sidecar | **New in M4.** How often the accrued hold time is flushed to the journal entry. A crash loses at most this much, in the safe direction (§9.3). |
| `maxReroutes` | `4` | sidecar | **New in M4.** Re-routes one entry may take before it bounces home instead (§9.2). |
| `forwardRecordRetentionSeconds` | `172800` | relay | **New in M4.** How long the relay remembers a forwarded `migrationId`, in memory, for the `neverForwarded` proof — 48 hours, twice the default hold (§5.2). |
| `inboundQueueMax` | `64` | sidecar | Shared with `contract-a.md` §10. Also the ceiling a paced backlog hits, which is what turns pacing into upstream backpressure (`contract-a.md` §7.5). |
| `exportRetentionSeconds` | `3600` | sidecar | Tombstone lifetime. Shared with `contract-a.md` §10. |
| `maxFrameBytes` | `8388608` | both | Shared with `contract-a.md` §10. |
| `maxPayloadBytes` | `4194304` | both | Shared with `contract-a.md` §10. Applies to `body.bb8` and to a `GENOME_RESPONSE` body. |
| `archiveQueueMax` | `1024` | relay | Copied frames buffered per subscriber before the oldest is dropped (§5.1). |
| `genomeRequestTimeoutMs` | `15000` | requester | How long a requester waits for a `GENOME_RESPONSE` before it counts the attempt as failed. **Requester-side only, and deliberately the only entry** — see the note below. |
| `genomeRequestsPerMinute` | `30` | both | Per requester, per answering peer. Enforced on both sides (§10). |
| `genomeCacheRetentionDays` | `30` | sidecar | Genome cache lifetime, least-recently-served. |
| `genomeCacheMaxBytes` | `2147483648` | sidecar | 2 GiB cap on `<data-dir>/genomes/`. |

The **delivery rate limit** is a Contract A tunable, because it paces a Contract A message:
`inboundRatePerSimMinute`, `inboundRateBurst` and `pacingIdleGraceMs` are in
`contract-a.md` §10. It is named here because its backpressure is this wire's `OVERLOADED`
and its side effects are this wire's hold clock.

**There is no answering-side timeout for `GENOME_REQUEST`, and its absence is deliberate.**
§6.9 says the answering sidecar must reply "within `genomeRequestTimeoutMs`", which reads
like a deadline it has to arm. It is not. The answer is served **synchronously** out of the
genome cache, the journal or the tombstones — a local read on data the sidecar already has —
so the handler either produces a `GENOME_RESPONSE` or produces
`found: false, reason: "unknown_hash"`, and it does both immediately. There is no wait to
time out. `genomeRequestTimeoutMs` is therefore a **requester-side** budget only, and a late
answer that arrives after it is still attributable through the echoed `genomeHash` (§6.10)
and may simply be used.

**Two defaults are worth a second look before the rig runs long.** `holdTimeoutMs` at 24
hours is the owner's call and is a policy, not a measurement — it trades a stranded organism
against the accepted duplication case (§9.3). `forwardRecordRetentionSeconds` at 48 hours is
sized from it: a record shorter than the hold means an entry can outlive the evidence that
would have let it re-route, which is safe and wasteful. Keep the second at least twice the
first.

---

## 13. Open items for M5

1. **No TLS, and one shared token** (§3.1). The wire is plain HTTP on a LAN, so a genome, a
   peer id and the token itself are readable in transit, and any token holder can present any
   `peerId`. M5 brings TLS and per-peer credentials together, because splitting them produces
   a half-secured relay that reads as secured. **Unchanged from M3, and now one milestone
   nearer.**
2. **A permanently rejected inbound organism is held, never returned** (§9.4). A safe
   two-phase return needs one more message pair. It stays parked while "held for an operator"
   remains an honest answer.
3. **Placement under churn is still only half-tested.** M4 places six known peers and splices
   one newcomer, by hand, once each. Strangers joining and leaving continuously is M5's
   problem, and it is where auto-placement, the coalescing window and `maxSlotEverIssued`
   growth will first be stressed.
4. **The archive has no write interface and no authentication of its own.** It is a
   subscriber that trusts the shared token. A public relay cannot copy every envelope to
   whoever asks, so M5 needs a subscriber authorisation rule — and the M4 stats block makes
   that sharper, because a copied `PEER_STATUS` now carries every world's population and save
   state.
5. **Release and handover are startup flags** (§7.5). If the map ever grows past what one
   operator can restart at will, both need an authenticated admin path — which is another
   reason they wait for the milestone that brings authentication.
6. **The forwarding record does not survive a relay restart** (§5.2). Every outstanding
   `sent` entry loses its chance of a proof at that moment and falls back to the bounded hold.
   That is correct, and it is also a real cost the first time a relay is restarted under load:
   entries that could have been re-routed in seconds wait out a 24-hour clock instead. A
   durable record is not the fix — a record that outlives the process would assert what the
   process cannot know. The fix, if this ever hurts, is a **receipt**: the relay
   acknowledging each forward to the sender, which turns the sender's own journal into the
   evidence and costs one frame per migration. M4 does not buy that, because a relay restart
   is rare and `--release-inflight` is one command.
7. **A stats block is unauthenticated telemetry from a peer.** The relay copies it verbatim
   into a broadcast every client reads. On a LAN of the owner's own machines that is fine; on
   a public relay a peer could report any population it liked, and the status page would show
   it. M5's per-peer credentials are the precondition for trusting any of it.
