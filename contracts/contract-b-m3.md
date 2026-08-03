# Contract B — M3 (Sidecar ↔ Relay ↔ Sidecar ↔ Archive)

**Version:** `contract-b/2.0`
**Status:** implementation-ready for M3. Written 2026-08-03 from the ratified decisions
D8–D11 (`system_decomposition.md`) and the work order in `m3_considerations.md`,
*Contract Changes Needed*.
**Supersedes:** `contracts/contract-b-m2.md`, in full. That document is the historical
record of the M2 rig and is **not** current guidance.
**Companion documents:** `contracts/contract-a.md` (`contract-a/1.1`, mod ↔ sidecar) and
`contracts/genome-hash.md` (`bb8-genome/1`, the canonical genome projection).

> ### Why a successor document and not an amendment
>
> `contract-a.md` was amended in place twice, because both amendment sets are
> clarifications and additive fields on top of a body that still describes the wire
> correctly. Contract B is a different case, and three things decide it:
>
> 1. **The M2 file is milestone-named and milestone-scoped.** It is titled *M2 Subset* and
>    its own §1 opens "M2 has exactly two sectors, `A` and `B`". A file called
>    `contract-b-m2.md` that describes the M3 ring would be misleading in its filename, its
>    title and its first paragraph — the three things a reader trusts before reading
>    anything else.
> 2. **The change is structural, not clarifying.** The ring replaces the sector map, the
>    claim and grant messages change shape, the envelope gains a lineage annex, two message
>    types are new, the transport gains authentication, and the relay gains a subscriber.
>    Half the message catalogue moves. An amendment set that large stops being an amendment
>    set and becomes a second document interleaved with the first, and a fresh implementer
>    then has to reconstruct the current wire by applying a diff in their head — which is
>    exactly the failure mode `contract-a.md` §13 exists to prevent, not to repeat.
> 3. **M2's text is still needed as it stands.** `m2_findings.md` and
>    `m2_considerations.md` cite it as the specification the passing M2 exit test ran
>    against. Overwriting it would destroy that record; leaving it beside a superseding
>    document keeps both readings honest.
>
> The rule for a reader is therefore simple: **this document is Contract B. The M2 file is
> history.** Nothing here inherits from it silently — where a rule carried over unchanged,
> it is restated here in full, and §11 lists everything that changed and why.

This document is written so a Go implementer building the relay, a Go implementer building
the sidecar, and a Go implementer building the archive can each build their side without
talking to each other. It reuses `contract-a.md`'s envelope, version rules and RFC 2119 key
words. Where Contract A already answers a question, this document points at it instead of
restating it.

---

## 1. Scope

In scope for M3: one ring of three slots across two physical machines on a LAN, one relay,
one archive, ring insertion keyed on peer identity, the lineage annex, genome fetch by
hash, and a shared token on the wire (D9).

Out of scope, and named so nobody builds them by accident: TLS and per-peer authentication
(M4), capacity and abuse limits (M4), `TOPOLOGY_GOSSIP` and `PEER_EXCHANGE` (M5),
`CATALOG_QUERY` and `CATALOG_RESPONSE` (M6), and any write interface on the archive — M3
records and reads only (D11).

---

## 2. Topology — the ring

M3 has one ring of three slots on two machines:

```
   ┌────────► slot 1 ────────► slot 2 ────────► slot 3 ────────┐
   │          (main)          (second)         (main)          │
   └───────────────────────────────────────────────────────────┘

   Every arrow is one lane: out of an east edge, into a west edge.
   Wiring is still a star: every sidecar dials the relay, the relay forwards.
   Main machine:    slot 1, slot 3, the relay, the archive.
   Second computer: slot 2.
```

| Term | Definition |
|---|---|
| **ring slot** | A position in the ring, identified by an integer `≥ 1`. It is the routing address of a peer. It replaces M2's sector. |
| **ring order** | The ordered list of slots. The east neighbour of an entry is the next entry; the east neighbour of the **last** entry is the **first**. This wrap-around is the ring. |
| **east neighbour** | The one slot a peer exports into. A peer has exactly one, and it is the whole topology a sidecar needs. |
| **slot reservation** | The binding of a slot to a `peerId`. It survives disconnection, restart and reinstall, and **it does not expire** (D8). Only an operator releases it (§7.5). |
| **lane** | One directed edge of the ring: source east edge → destination west edge. Lanes are one-way; there is no return lane. |

Three properties follow, and every rule in this document depends on them:

- **A slot belongs to a peer identity, not to a connection.** An offline peer keeps its
  slot; its lane simply closes (§8).
- **The map never reshuffles.** Insertion appends; release splices out. No surviving slot
  ever changes its number, and no pair of surviving slots ever changes its relative order
  (§7).
- **Out and in are different doors.** An organism that leaves through the east edge cannot
  come back through the same edge; it must travel the whole ring. There is no boomerang at
  a shared edge, and no ping-pong to time out.

**Degenerate ring sizes.** With `ringSize = 1` a peer's east neighbour would be itself; the
relay **MUST NOT** grant that peer an east neighbour, and its export edge stays closed with
`no_peer`. With `ringSize = 2` the ring is legal and works — `A → B → A` — but one hop is
already a return trip, so the M3 exit test needs three slots
(`m3_considerations.md`, *Purpose*).

---

## 3. Transport

| Property | Value |
|---|---|
| Protocol | WebSocket (RFC 6455) over plain HTTP. TLS is M4 (D9). |
| URL | `ws://{relay-host}:{port}/contract-b/v2` |
| Default port | `8790` |
| Bind address | The relay binds a LAN-reachable address in M3, **not** loopback. The operator opens the Windows Firewall rule for the port and records the host name in `dev_environment.md` (`m3_considerations.md`, Risk 6). |
| Roles | The **relay is the server**. Every sidecar and the archive are **clients** and do all the dialling. |
| Frame type | Text frames. One JSON object per frame. No batching. |
| Encoding | UTF-8, no BOM |
| Compression | `permessage-deflate` **MUST NOT** be negotiated |
| Max frame size | 8 MiB (`maxFrameBytes`), same as Contract A |
| Authentication | A **shared bearer token** on the HTTP upgrade — §3.1 |
| Reconnect | Exponential backoff with full jitter, `relayBackoffMinMs` to `relayBackoffMaxMs`, the same rule Contract A §6.2 gives the mod. The ladder resets only after a session that stayed up for `stableSessionMs` (Contract A §13, A8). |

### 3.1 The LAN token

M2 ran on loopback and needed no authentication. M3's wire crosses a LAN, so any device on
that network can reach the relay port. The answer is deliberately the smallest one that
works, because **M3 is not the public milestone** — D9 moved TLS, per-peer identity and
abuse limits to M4 as one coherent set.

| Rule | Statement |
|---|---|
| Where | The HTTP request that opens the WebSocket carries `Authorization: Bearer <token>`. Nothing token-related appears in any frame. |
| Who | Every client: both sidecars, the third sidecar, and the archive. One token for the whole ring. |
| Source | The environment variable `MULTIVERSE_TOKEN` on both sides, or `--token-file <path>` which reads the first line and strips trailing whitespace. A flag that takes the token literally **MUST NOT** exist — it would put the secret in every process listing. |
| Shape | 16 to 256 bytes of printable ASCII. The RECOMMENDED value is 32 random bytes, hex-encoded. |
| Comparison | Constant-time (`crypto/subtle.ConstantTimeCompare`). Never `==`. |
| Missing or wrong | The relay answers HTTP **401** with `WWW-Authenticate: Bearer` and **does not upgrade**. There is no WebSocket, so there is no close code. |
| No token configured | The relay **MUST** refuse to start, unless `--insecure-no-token` is passed, in which case it logs one loud warning per accepted connection. The flag exists for a single-machine test rig and is never used on the LAN. |
| Client reaction to 401 | Retry on the normal backoff ladder, and log one loud error each time. After `authFailuresBeforeCeiling` (5) consecutive 401s, hold the backoff at `relayBackoffMaxMs`: a wrong token is an operator problem and hammering the relay will not fix it. |

**What this does and does not buy.** It keeps an unrelated device on the LAN out of the
ring. It does **not** authenticate a peer *identity*: a token holder can present any
`peerId`, including one that already holds a slot, and the `4006` rule (§3.2) will then
evict the legitimate peer. It does **not** provide confidentiality — the wire is plain
HTTP, so anything on the LAN path can read a genome, a token or a peer id in transit. Both
gaps are accepted for a milestone whose entire network is two computers the owner owns, and
both are closed by M4's TLS and per-peer credentials.

### 3.2 Close codes

| Code | Name | Sent by | Meaning |
|---|---|---|---|
| `1000` | `NORMAL` | either | Clean shutdown. |
| `1009` | `TOO_BIG` | either | Frame over `maxFrameBytes`. |
| `4000` | `PROTOCOL_UNSUPPORTED` | relay | The `protocol` **major** version is not supported. The client **MUST NOT** reconnect until it is restarted. |
| `4003` | `MALFORMED_FRAME` | either | Not valid JSON, a missing REQUIRED envelope field, no `HANDSHAKE` first, or a routing field that disagrees with the sender's own peer id. Reconnect with backoff. |
| `4004` | `LIVENESS_TIMEOUT` | relay | No frame and no `PONG` within `peerTimeoutMs`. Reconnect with backoff. |
| `4005` | `SHUTTING_DOWN` | either | The sender is draining. Reconnect with backoff. |
| `4006` | `REPLACED` | relay | A newer connection claimed the same `peerId`. The old connection **MUST NOT** reconnect. |

---

## 4. The envelope

Identical in shape to Contract A §3 — five fields, no more:

```json
{
  "protocol": "contract-b/2.0",
  "type": "MIGRATION_PAYLOAD",
  "messageId": "b7d1e0c4-9f2a-4c31-8b6d-2e0a41f5c7a9",
  "sentAt": 1785693600123,
  "data": { }
}
```

Contract A §3.1 and §3.2 apply unchanged, including the M3 minor-version rule
(`contract-a.md` §14, A16): the version segment is `<major>.<minor>`, compatibility is on
the **major** only, the minor is never a rejection reason, changes within a major are
additive fields and additive enum values only, unknown fields and unknown types are
ignored, and a malformed frame closes with `4003`.

**This is a major bump from M2's `contract-b/1`.** Work order item 10 asks for it and it is
correct here: the `["A", "B"]` sector set is deleted, `sourceSector` and `destSector` change
type, and `SECTOR_GRANT.sector` is replaced. A `contract-b/1` sidecar and a `contract-b/2`
relay are incompatible by design and say so with close `4000` instead of misrouting an
organism. Contract A took a **minor** bump for the same milestone, and `contract-a.md` §14
A16 explains why the two differ.

Timestamps are informational (D5). `messageId` is for log correlation only; `migrationId`
is the one idempotency key in the system (Contract A §7.1).

---

## 5. Routing, and what the relay may read

The relay reads exactly two fields out of `data` and nothing else:

| Field | Present on | Routes to |
|---|---|---|
| `destSlot` | `MIGRATION_PAYLOAD` | The peer that currently holds that ring slot |
| `destPeer` | `MIGRATION_ACK`, `MIGRATION_NACK`, `GENOME_REQUEST`, `GENOME_RESPONSE` | That peer id |

Every routable frame also carries `sourcePeer`. The relay **MUST** check it against the
sending connection's own peer id and **MUST** close with `4003` when they disagree. It
**MUST NOT** rewrite it, so the frame can be forwarded byte for byte.

The relay **MUST NOT** decode `data.body.bb8`, **MUST NOT** decode `data.lineage`, and
**MUST NOT** validate a payload (D4 — that is `bb8-schema`'s job, sidecar-side, at both
ends). It forwards the original frame bytes unchanged.

**Routing is on the slot, not on the peer.** `destSlot` is an address: if the peer holding
that slot changed identity since the sender journaled the migration, the frame goes to
whoever holds the slot now. This is what makes an insertion safe for in-flight work (§7.3).

When the destination slot has no live peer, or `destPeer` is not connected, the relay
**MUST** answer the *sender* — `MIGRATION_NACK` with `SLOT_VACANT` or `PEER_UNKNOWN` for a
migration, `GENOME_RESPONSE` with `found: false, reason: "peer_offline"` for a fetch —
rather than drop the frame. A dropped frame turns a bounded failure into a stall.

### 5.1 The archive is a read-only subscriber

`multiverse-archive` connects to the relay as a client with `role: "archive"` (§6.1). It
owns no world, holds no ring slot, and never appears in the ring order.

| Rule | Statement |
|---|---|
| Fan-out | The relay **MUST** send every connected subscriber a **byte-identical copy** of every `MIGRATION_PAYLOAD` it routes, and of every `MIGRATION_ACK` and `MIGRATION_NACK` it routes. The copy carries the original `sourcePeer`, `destSlot` and `migrationId`. |
| Best effort | The fan-out **MUST NOT** delay, block or fail a migration. A subscriber that is absent, slow or dead changes nothing on the migration path. |
| Bounded | Each subscriber has a queue of `archiveQueueMax` (1024) frames. On overflow the relay **MUST** drop the **oldest** copy, increment a dropped-copies counter, and log at most one line per minute. It **MUST NOT** disconnect the migration it was copying. |
| No answers | A subscriber **MUST NOT** answer a copied frame. The relay ignores an `ACK`/`NACK` from a subscriber with one warning. |
| No sending | A `MIGRATION_PAYLOAD` from a subscriber is answered `MIGRATION_NACK` / `NOT_A_MEMBER` and is **not** forwarded. |
| What a subscriber may send | `HANDSHAKE`, `PING`, `PONG`, `GENOME_REQUEST`. Nothing else. |
| No claim | A `SECTOR_CLAIM` from a subscriber is refused with `granted: false, reason: "role_has_no_slot"`. |
| Duplicates | A re-forwarded migration produces a second copy. The archive deduplicates on `migrationId`, exactly as a sidecar does. |

**This resolves owner design call 2** (`m3_considerations.md`, *Design Calls for the
Owner*) in favour of the relay copy, over the alternative of the archive joining as a
slot-less ring member. The reasons:

- A slot-less ring member has to be special-cased in every rule that says "each peer has
  one east neighbour", in the insertion protocol, and in the export-edge ripple. The copy
  is one rule in the router.
- D1's "dumb relay" survives intact under either option, and it is what makes the copy
  cheap: the relay does not parse a body, does not index a genome and does not learn what a
  lineage annex is. It appends bytes it was already forwarding to one more socket.
- The archive stays outside the migration path, which is Risk 7's requirement: nothing in a
  migration ever waits for the archive.

The reversal cost is bounded and named, so this decision can be revisited: if the archive
ever needs to be a ring participant, the fan-out rule is deleted and the insertion protocol
grows a role check.

---

## 6. Message catalogue

Twelve types. Two are new in M3 (`GENOME_REQUEST`, `GENOME_RESPONSE`).

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

The two claim messages keep their M2 names. The noun changed — a sector is now a ring slot
— but `system_decomposition.md`'s ratified Contract B message list still names
`SECTOR_CLAIM`, and renaming a ratified message to rename a noun buys less than it costs.
§11 item 1 records the choice.

### 6.1 `HANDSHAKE` — client → relay

The **first frame on every connection**. Any other first frame closes with `4003`.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `peerId` | string | yes | Stable identity of this client. `1`–`64` characters, `[A-Za-z0-9._-]`. It is what makes a slot reclaim work across a restart, so it **MUST** be persisted (§7.4). |
| `role` | string enum | yes | `"peer"` — owns a world and a ring slot — or `"archive"` — a read-only subscriber (§5.1). New in M3. |
| `protocolVersion` | string | yes | `"contract-b/2.0"`. A different **major** closes with `4000`. |
| `gameVersion` | string | yes | The game version behind this sidecar, from the mod's `CONFIG_UPDATE`. Empty while no mod is connected, and always empty for an archive. |
| `sidecarVersion` | string | yes | Informational. The archive sends its own version here. |
| `simulationSize` | float | no | `S`, when a mod has already reported one. |

A second connection presenting a `peerId` that is already live **MUST** cause the relay to
close the older connection with `4006` and serve the newer one — the same self-healing rule
Contract A §2 gives the mod socket.

**Compatibility enforcement at connect.** The relay **MUST** refuse a peer whose
`gameVersion` is incompatible with the ring's, and it **MUST** be loud about it: it closes
with `4003`, logs one error naming both versions, and reports the refusal in the next
`PEER_STATUS` as a vacant slot with `lastRefusal`. A silent version mismatch is
indistinguishable from a dead peer — both end with a closed export edge — and M3 crosses
two independently updated installs, so this is the failure most likely to waste an evening
(`m3_considerations.md`, Risk 5). An **empty** `gameVersion` is not a mismatch: it means no
mod is connected yet.

```json
{
  "protocol": "contract-b/2.0",
  "type": "HANDSHAKE",
  "messageId": "9d1a4b77-2c60-4c1e-9f03-77a1c8e4b510",
  "sentAt": 1785693597011,
  "data": {
    "peerId": "peer-lan-slot2",
    "role": "peer",
    "protocolVersion": "contract-b/2.0",
    "gameVersion": "0.6.3.1",
    "sidecarVersion": "0.3.0",
    "simulationSize": 2000.0
  }
}
```

### 6.2 `HANDSHAKE_ACK` — relay → client

| Field | Type | Required | Semantics |
|---|---|---|---|
| `relayVersion` | string | yes | Informational. |
| `protocolVersion` | string | yes | `"contract-b/2.0"`. |
| `assignedSlot` | number (int) | no | The slot this `peerId` already holds, when the relay remembers one. Absent for a first-time peer and always absent for an archive. |
| `ringSize` | number (int) | yes | How many slots are in the ring right now. `0` before the first insertion. |
| `receivedAt` | `timestampMs` | yes | The relay's own clock. Informational, and the anchor the archive uses to order records written by two machines' clocks (`m3_considerations.md`, Risk 5). |

```json
{
  "protocol": "contract-b/2.0",
  "type": "HANDSHAKE_ACK",
  "messageId": "0b4e2a13-5d77-4b90-8a21-6f0c19d4e772",
  "sentAt": 1785693597019,
  "data": {
    "relayVersion": "0.3.0",
    "protocolVersion": "contract-b/2.0",
    "assignedSlot": 2,
    "ringSize": 3,
    "receivedAt": 1785693597018
  }
}
```

### 6.3 `SECTOR_CLAIM` — sidecar → relay

A **ring claim**. Sent right after `HANDSHAKE`, and again whenever `simulationSize`,
`exportEdge` or `gameVersion` change. A repeat claim from a peer that already holds a slot
is an **update**, never a second claim.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `preferredSlot` | number (int) | no | The slot this sidecar held last time, replayed from `<data-dir>/slot` (§7.4). Advisory. `≥ 1`. |
| `simulationSize` | float | yes | `S`, as last reported by the mod. `0` while no mod is connected. |
| `exportEdge` | edge enum | yes | The mod's declared export edge, from Contract A `CONFIG_UPDATE`. `"E"` under the ring. `""` while no mod is connected. |
| `borderEdges` | array of edge | yes | The mod's declared strips — the edges that accept an inbound organism. `["E","W"]` under the ring, empty while no mod is connected. |
| `gameVersion` | string | no | Updates the value from `HANDSHAKE`. |
| `modConnected` | bool | yes | Whether a mod is connected right now. A sidecar with no mod cannot spawn an organism, and its west neighbour's export edge closes (§8). |

```json
{
  "protocol": "contract-b/2.0",
  "type": "SECTOR_CLAIM",
  "messageId": "4c7f0d92-8a11-4e63-bb05-2d971a0c3e44",
  "sentAt": 1785693597033,
  "data": {
    "preferredSlot": 2,
    "simulationSize": 2000.0,
    "exportEdge": "E",
    "borderEdges": ["E", "W"],
    "gameVersion": "0.6.3.1",
    "modConnected": true
  }
}
```

### 6.4 `SECTOR_GRANT` — relay → sidecar

The grant returns the slot **and** the east neighbour. Together they are the entire topology
a sidecar needs (D8).

| Field | Type | Required | Semantics |
|---|---|---|---|
| `granted` | bool | yes | |
| `slot` | number (int) | no | Present when `granted` is `true`. |
| `ringSize` | number (int) | yes | Slots in the ring after this grant. |
| `reason` | string enum | yes | `"granted"` (a new slot was inserted), `"reclaimed"` (the reservation for this `peerId` was still held), `"updated"` (a repeat claim), `"role_has_no_slot"`, `"protocol_mismatch"`, `"version_incompatible"`. |
| `eastNeighbour` | object | no | Present when `granted` is `true` **and** `ringSize ≥ 2`. Absent at `ringSize = 1`, where a peer has no east neighbour but itself. |
| `eastNeighbour.slot` | number (int) | yes | The slot this peer exports into. |
| `eastNeighbour.peerId` | string | yes | The identity reserved to that slot, live or not. |
| `eastNeighbour.live` | bool | yes | Whether that peer is connected right now. |
| `eastNeighbour.modConnected` | bool | yes | Whether that peer has a mod. |
| `eastNeighbour.gameVersion` | string | yes | For the compatibility test. |
| `eastNeighbour.simulationSize` | float | yes | For the `S` test of Contract A §13, A10. |

A refused claim carries `granted: false` and no `slot`. The sidecar keeps its export edge
closed and retries on the next `PEER_STATUS` change.

```json
{
  "protocol": "contract-b/2.0",
  "type": "SECTOR_GRANT",
  "messageId": "e2b90c47-1f35-4d02-9c68-51a7d3b0f981",
  "sentAt": 1785693597041,
  "data": {
    "granted": true,
    "slot": 2,
    "ringSize": 3,
    "reason": "reclaimed",
    "eastNeighbour": {
      "slot": 3,
      "peerId": "peer-main-slot3",
      "live": true,
      "modConnected": true,
      "gameVersion": "0.6.3.1",
      "simulationSize": 2000.0
    }
  }
}
```

### 6.5 `PEER_STATUS` — relay → client

**Full state, not a delta**, exactly like Contract A's `EDGE_STATUS`. Sent after every
registry change: a peer connecting or dying, a slot granted or released, a
`simulationSize` update, a mod connecting or disconnecting behind a peer.

`PEER_STATUS` reports **the ring order, not a peer list** (work order item 2). A slot with
no live peer stays in the ring with `live: false`; it does not disappear.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `epoch` | number (int64) | yes | Strictly increasing per connection, from 1. A receiver **MUST** ignore an epoch lower than or equal to the last applied one. Resets on a new connection. |
| `ringSize` | number (int) | yes | Number of entries in `slots`. |
| `slots` | array of object | yes | **In ring order.** The east neighbour of entry `i` is entry `i+1`, and the east neighbour of the last entry is the first. |
| `slots[].slot` | number (int) | yes | The slot number. Stable for the life of the reservation. |
| `slots[].peerId` | string | yes | The reserved identity. Never empty — a slot exists only because a peer claimed it. |
| `slots[].live` | bool | yes | Whether that peer has a live relay connection. |
| `slots[].modConnected` | bool | yes | Whether that peer has a live mod. |
| `slots[].gameVersion` | string | yes | Empty when unknown. |
| `slots[].simulationSize` | float | yes | `0` when unknown. |
| `slots[].lastSeenMs` | `timestampMs` | no | Relay clock, last frame from that peer. Informational, for the operator. |
| `slots[].lastRefusal` | string | no | Present when the relay last refused this peer's connection, naming the reason (§6.1). This is how a version mismatch stops looking like a dead peer. |
| `you` | object | yes | `{"slot": int|null, "eastNeighbourSlot": int|null}` for the receiving client. Both are `null` for a subscriber and at `ringSize` 1. |
| `observers` | number (int) | yes | Connected read-only subscribers. Informational. |

```json
{
  "protocol": "contract-b/2.0",
  "type": "PEER_STATUS",
  "messageId": "77c0e1a4-63b8-4f19-8d2a-9e40b7c15206",
  "sentAt": 1785693598002,
  "data": {
    "epoch": 4,
    "ringSize": 3,
    "slots": [
      { "slot": 1, "peerId": "peer-main-slot1", "live": true,  "modConnected": true,
        "gameVersion": "0.6.3.1", "simulationSize": 2000.0, "lastSeenMs": 1785693597998 },
      { "slot": 2, "peerId": "peer-lan-slot2",  "live": true,  "modConnected": true,
        "gameVersion": "0.6.3.1", "simulationSize": 2000.0, "lastSeenMs": 1785693597991 },
      { "slot": 3, "peerId": "peer-main-slot3", "live": false, "modConnected": false,
        "gameVersion": "0.6.3.1", "simulationSize": 2000.0, "lastSeenMs": 1785693511004 }
    ],
    "you": { "slot": 2, "eastNeighbourSlot": 3 },
    "observers": 1
  }
}
```

In that example slot 3 is reserved but offline. Slot 2's export edge therefore closes
(`no_peer`), slot 3's own west lane is irrelevant while it is away, and **slot 1 is
unaffected** — the ripple is one-way (§8).

### 6.6 `MIGRATION_PAYLOAD` — sidecar → sidecar, forwarded

The Contract C `MigrationEnvelope`, carried in `data`, now with the lineage annex (D11).

| Field | Type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The idempotency key (D2). Minted by the origin **mod**, preserved end to end. |
| `kind` | string enum | yes | `"bibite"` in M3. Anything else is answered `MIGRATION_NACK` / `KIND_UNSUPPORTED`. |
| `body.version` | string | yes | The game version that serialized the blob. Authoritative over the blob's own `version` key (Contract A §4.6). |
| `body.bb8` | string | yes | The opaque blob, as a JSON **string**, never nested, never base64. Max `maxPayloadBytes` (4 MiB). |
| `lineage` | object | yes | The annex. Always present; `parents` may be empty. |
| `lineage.genomeHash` | string | yes | The migrant's own genome hash, computed by the **source sidecar** from `body.bb8` with `genome-hash.md`. The archive's join key. |
| `lineage.parents` | array of object | yes | `0`–`2` entries, in `genes.parent1` then `genes.parent2` order. `[]` is normal. |
| `lineage.parents[].entityId` | `entityId` | yes | The parent's id, from the migrant's genes. Signed int32, often negative. |
| `lineage.parents[].genomeHash` | string | no | The parent's genome hash. **Absent means a gap** — the parent genome was not available to hash. |
| `lineage.parents[].gapReason` | string enum | no | Present exactly when `genomeHash` is absent: `"parent_gone"` (no blob was shipped — the usual case), `"blob_invalid"` (`bb8-schema` could not hash it), `"blob_dropped_for_size"` (the mod trimmed it to fit the frame). |
| `sourcePeer` | string | yes | Origin peer id. The relay verifies it. |
| `sourceSlot` | number (int) | yes | The origin's ring slot. |
| `destSlot` | number (int) | yes | The origin's **east neighbour** slot at the moment the migration was journaled. The relay routes on this. |
| `exitEdge` | edge enum | yes | Always `"E"` under the ring. Kept as an enum for the retired grid and M6's payload kinds. |
| `exitPosition` | float | yes | `[0,1]` along that edge, by Contract A §4.3. Already clamped by the origin mod — the capture band makes an unclamped raw value routine (Contract A §4.3.1). |
| `velocity` | `{x,y}` | yes | Copied, never mirrored (Contract A §4.4). |
| `heading` | float | yes | Degrees (Contract A §4.4). |
| `entityId` | `entityId` | yes | Signed int32, often negative. See §11 item 3 for which value wins. |
| `timestamp` | `timestampMs` | yes | The origin's clock. Informational (D5). |

**The wire never carries a parent blob.** The mod ships opaque parent blobs on Contract A
(`contract-a.md` §14, A12); the source sidecar hashes them, caches them under their hashes,
and **strips them here**. A genome travels a second time only in answer to a
`GENOME_REQUEST` (§6.9). Three parent blobs on every envelope would triple the wire cost of
a migration to carry data almost every receiver already has.

`entryEdge` is not a field. The receiving sidecar derives it: `"W"` for ordinary ring
traffic — the passive entry edge — and its own peer's `exportEdge` for a bounce-back
(§9, `contract-a.md` §14 A11).

The receiving sidecar **MUST**, in this order:

1. Deduplicate on `migrationId` against its journal **and its tombstones**. A hit is
   answered `MIGRATION_ACK` immediately and delivered nothing (Contract A §7.2).
2. Apply admission control. More than `inboundQueueMax` (64) un-delivered journal entries
   is answered `MIGRATION_NACK` / `OVERLOADED`.
3. Check `S` against its own, by the relative test of Contract A §13, A10. A mismatch is
   `MIGRATION_NACK` / `SIM_SIZE_MISMATCH`.
4. Validate the blob with `bb8-schema` against `body.version`. Failure is
   `MIGRATION_NACK` / `INVALID_PAYLOAD`.
5. Write the journal entry and **flush it to durable storage**. Custody moves here.
6. Cache the migrant's genome under `lineage.genomeHash`, so this peer can serve it later
   (§10). A cache write failure is logged and never fails the migration.
7. Deliver it to its mod as Contract A `MIGRATE_IN`, replaying until the mod ACKs
   (Contract A §7.5).

**The receiver never recomputes `lineage.genomeHash` as a gate.** It MAY recompute it for a
consistency log line, but a mismatch is a `bb8-schema` defect to shout about, not a reason
to refuse an organism — custody rules outrank bookkeeping.

```json
{
  "protocol": "contract-b/2.0",
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
    "sourcePeer": "peer-main-slot1",
    "sourceSlot": 1,
    "destSlot": 2,
    "exitEdge": "E",
    "exitPosition": 1.0,
    "velocity": { "x": 61.2, "y": 4.4 },
    "heading": 274.11,
    "entityId": -843827577,
    "timestamp": 1785693600149
  }
}
```

### 6.7 `MIGRATION_ACK` — sidecar → sidecar, forwarded

**Sent only after the receiving mod's Contract A `MIGRATE_IN_ACK`.** This is the link in
the custody chain that lets the origin sidecar drop its own journal entry (D2, custody
chain step 5). A sidecar **MUST NOT** send `MIGRATION_ACK` when it has merely journaled the
payload.

The one exception is the deduplication path of §6.6 step 1: a `migrationId` that is already
tombstoned was ACKed once before, so re-ACKing it carries the same meaning.

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
  "protocol": "contract-b/2.0",
  "type": "MIGRATION_ACK",
  "messageId": "58d2c0b9-3417-4a6f-9e28-b1d05c7a2f33",
  "sentAt": 1785693600402,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "sourcePeer": "peer-lan-slot2",
    "destPeer": "peer-main-slot1",
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

| Code | Class | Sent by | Cause |
|---|---|---|---|
| `SLOT_VACANT` | transient | relay | No live peer holds `destSlot`. Renamed from M2's `SECTOR_VACANT`. |
| `PEER_UNKNOWN` | transient | relay | `destPeer` is not connected. |
| `NOT_A_MEMBER` | permanent | relay | The sender is a read-only subscriber and may not send migrations (§5.1). New in M3. |
| `OVERLOADED` | transient | sidecar | `inboundQueueMax` reached. |
| `SIM_SIZE_MISMATCH` | transient | sidecar | The two sims disagree about `S`. |
| `MOD_ABSENT` | transient | sidecar | No mod is connected and the queue is full. |
| `INVALID_PAYLOAD` | permanent | sidecar | `bb8-schema` rejected the blob, or it is over `maxPayloadBytes`. |
| `KIND_UNSUPPORTED` | permanent | sidecar | `kind` is not `"bibite"`. |
| `VERSION_UNSUPPORTED` | permanent | sidecar | No `bb8-schema` dialect for `body.version`. |
| `MALFORMED_MESSAGE` | permanent | sidecar | A `data` field failed validation. |
| `SHUTTING_DOWN` | transient | either | The sender is draining. |

**A sidecar MUST NOT send `MIGRATION_NACK` after it has durably journaled the payload.**
This is what makes a NACK a *definitive* statement that custody never moved, and it is what
makes the origin's bounce-back safe. See §9.

**No code in this table is ever caused by the lineage annex.** A missing hash, a gap, an
unknown `gapReason` or an empty `parents` array is never a reason to refuse an organism.
The annex is bookkeeping; the organism is custody.

```json
{
  "protocol": "contract-b/2.0",
  "type": "MIGRATION_NACK",
  "messageId": "b3160fe2-95ad-4c77-8f10-2a4e6c9b0715",
  "sentAt": 1785693600166,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "sourcePeer": "",
    "destPeer": "peer-main-slot1",
    "code": "SLOT_VACANT",
    "class": "transient",
    "message": "ring slot 2 is reserved to peer-lan-slot2, which has not been seen for 88s",
    "retryAfterMs": 15000
  }
}
```

### 6.9 `GENOME_REQUEST` — archive or sidecar → sidecar, forwarded

**New in M3 (D11).** The requester has a genome hash from an annex and no genome behind it.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `requestId` | `uuid` | yes | Correlates the answer. Not an idempotency key — a repeated request is simply answered again. |
| `sourcePeer` | string | yes | The requester. The relay verifies it. |
| `destPeer` | string | yes | The peer being asked. The relay routes on this. |
| `genomeHash` | string | yes | A full `genome-hash.md` value, label included. Never truncated. |
| `context` | object | no | Informational, for the answering peer's logs: `{"migrationId": uuid, "entityId": entityId}` — the annex the hash came from. |

**Who answers.** The peer named in `destPeer`. A requester **SHOULD** ask the envelope's
`sourcePeer` first, because that sidecar hashed and cached the blob (§10). If that
answer is `unknown_hash`, or the peer is offline, the requester MAY ask other live peers in
ring order, one at a time, honouring the rate limit.

**Answering obligations.** The answering sidecar **MUST** reply with exactly one
`GENOME_RESPONSE` within `genomeRequestTimeoutMs`, from its genome cache, its journal or
its tombstones. It **MUST NOT** block a migration to serve one, and it **MUST NOT** treat a
request it cannot serve as an error.

```json
{
  "protocol": "contract-b/2.0",
  "type": "GENOME_REQUEST",
  "messageId": "6a20f7c8-4b3d-4e51-9017-c8b25d0a4f16",
  "sentAt": 1785693605010,
  "data": {
    "requestId": "d4c19b06-7f52-4a83-b0e1-9c3d7a52f408",
    "sourcePeer": "archive-main",
    "destPeer": "peer-main-slot1",
    "genomeHash": "bb8-genome/1:sha256:43ea8315112301799622f3ab8906c2882e528502bc35424a209cd67bfdaec207",
    "context": {
      "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
      "entityId": -1180911975
    }
  }
}
```

### 6.10 `GENOME_RESPONSE` — sidecar → requester, forwarded

Also generated **by the relay** when it cannot route the request.

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

`"unknown_hash"` is a normal answer, not an error. `m3_considerations.md` Risk 7 lists the
four ways it happens: the source peer is offline, the parent never migrated so no journal
holds it, the source restarted and lost its cache, or the ring is busy and the request was
shed.

```json
{
  "protocol": "contract-b/2.0",
  "type": "GENOME_RESPONSE",
  "messageId": "3f7b1e05-0a94-4d2c-91b7-6d08e5c3a220",
  "sentAt": 1785693605033,
  "data": {
    "requestId": "d4c19b06-7f52-4a83-b0e1-9c3d7a52f408",
    "sourcePeer": "peer-main-slot1",
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

A rate-limited refusal:

```json
{
  "protocol": "contract-b/2.0",
  "type": "GENOME_RESPONSE",
  "messageId": "c0894a71-5b3e-4f60-88d2-71ae4c0b9d35",
  "sentAt": 1785693605041,
  "data": {
    "requestId": "8b21c7f4-90de-4a15-b6c3-0f5a7d219e64",
    "sourcePeer": "peer-main-slot1",
    "destPeer": "archive-main",
    "genomeHash": "bb8-genome/1:sha256:9f2c47a1b8e50d63cc41f0a7e29b5d38470c6a1eb92f8d05374ac6be1082fd57",
    "found": false,
    "reason": "rate_limited",
    "retryAfterMs": 60000
  }
}
```

### 6.11 `PING` / `PONG`

`data` is `{"nonce": "<uuid>"}` on `PING` and the same nonce on `PONG`. Either side may
ping. The relay pings every `relayPingIntervalMs` and closes a peer with `4004` after
`peerTimeoutMs` of complete silence. WebSocket-level ping/pong frames are used as well; the
application-level pair exists so a sidecar can measure the relay path itself — which now
crosses a real network hop.

---

## 7. Ring insertion

The relay is the single arbiter of the ring (D1, D8). No sidecar chooses its own slot, and
no sidecar learns the ring from anywhere else.

### 7.1 The model

The relay holds an **ordered list** of reservations:

```
ring = [ {slot: 1, peerId: "peer-main-slot1"},
         {slot: 2, peerId: "peer-lan-slot2"},
         {slot: 3, peerId: "peer-main-slot3"} ]

east(i) = ring[(i + 1) mod len(ring)]
```

Slot numbers are **identifiers**, not positions. The list is the ring order. A slot number
is never reused while its reservation exists and is never renumbered — that is what makes
"the map never reshuffles" true.

### 7.2 Arbitration rules

On `SECTOR_CLAIM` from a peer, in this order:

1. **This `peerId` already holds a slot** → it keeps it. `reason: "reclaimed"` on the first
   claim of a connection, `"updated"` on a repeat. The reservation is keyed on `peerId`,
   never on a connection, and **never expires**.
2. **`preferredSlot` names a slot reserved to this same `peerId`** → same as rule 1. (This
   is the ordinary case after a sidecar restart: the peer replays the slot it persisted.)
3. **`preferredSlot` names a slot that exists and is reserved to somebody else** → ignore
   the preference and fall through. A preference never evicts anybody.
4. **Otherwise, insert.** A new slot is created with `slot = maxSlotEverIssued + 1` and
   **appended to the tail** of the ring order. `reason: "granted"`.
5. **A claim from a `role: "archive"` client** → `granted: false`,
   `reason: "role_has_no_slot"` (§5.1).
6. **A claim from a peer the relay refused on compatibility grounds** → `granted: false`,
   `reason: "version_incompatible"`, and the refusal appears in `PEER_STATUS`.

**Why append at the tail.** Appending changes exactly one existing lane: the old tail's east
neighbour becomes the new peer instead of the head. Inserting anywhere else changes two,
and inserting *between two live peers* is what Risk 4 warns about. The ring is symmetric
under rotation, so the tail is as good a position as any and costs the least churn.

After any change the relay **MUST** broadcast a new `PEER_STATUS` with a higher `epoch` to
every client, and **MUST** send a fresh `SECTOR_GRANT` to any peer whose east neighbour
changed.

### 7.3 An insertion must not disturb work in flight

Three rules keep an insertion safe, and all three come from Risk 4:

1. **An insertion applies to new migrations only.** A journaled outbound entry keeps the
   `destSlot` it recorded. The sidecar **MUST NOT** rewrite a journal entry's destination
   because its east neighbour changed.
2. **Routing is on the slot** (§5). If the recorded `destSlot` still exists, the frame is
   delivered to whoever holds it. If it was released, the sender gets `SLOT_VACANT` and the
   organism bounces home (§9) — a bounded, correct outcome.
3. **A new `peerId` is a new peer.** A sidecar that is reinstalled with a fresh `peerId`
   takes a **new** slot, and its old slot stays reserved forever, holding its lane closed.
   That is why `peerId` is persisted outside the journal (§7.4) and why the operator needs
   a release command (§7.5).

### 7.4 What each side persists

| Side | File | Contents | Lost ⇒ |
|---|---|---|---|
| relay | `<data-dir>/ring.json` | The ordered reservation list, and `maxSlotEverIssued` | The whole ring is forgotten; every peer is inserted again as a new slot in connect order. **This is why it is durable in M3 and was not in M2**: a reservation that never expires is worthless if it lives in RAM. |
| sidecar | `<data-dir>/peer-id` | One line, the `peerId` | The peer becomes a stranger and takes a second slot, stranding its old one. Generated once, on first start, if absent. |
| sidecar | `<data-dir>/slot` | One line, the granted slot number | Only the `preferredSlot` hint; rule 1 recovers the slot from `peerId` anyway. |
| sidecar | journal | Custody, tombstones, recorded `destSlot` | Organisms. This is the one file whose loss is not recoverable, and D2 accepts that as loss. |

The relay writes `ring.json` **before** it answers a `SECTOR_CLAIM` that created or changed
a reservation — an answered grant that is not on disk can hand the same slot to two peers
across a restart. The sidecar writes `<data-dir>/slot` on every granted `SECTOR_GRANT` and
reads it once at startup; an explicit `--slot` flag overrides it, and an unreadable value is
treated as absent. Neither file belongs in the journal: losing them costs a slot mix-up,
never an organism.

### 7.5 Releasing a slot by hand

The reservation never expires, so the ring needs one operator escape hatch and exactly one:

- `multiverse-relay --release-slot <n>` at startup removes slot `n` from `ring.json`,
  splices it out of the ring order, and logs the change. Surviving slots keep their
  numbers and their relative order.
- The slot number is **not** reused: `maxSlotEverIssued` never decreases. A returning peer
  with the released `peerId` is inserted at the tail as a new slot.
- Releasing a slot whose peer is live is a mis-operation. The relay refuses it and says so.

There is no wire message for release, and there is no admin socket. An operator command is
a rare, deliberate, physical act on the relay's own machine, and giving it a network
surface in the milestone that first leaves loopback is a poor trade.

---

## 8. From `PEER_STATUS` to `EDGE_STATUS` — the one-way ripple

A sidecar's only job with the ring is to decide whether **its own export edge** is open, and
to tell its mod (`contract-a.md` §5.4). The mapping is exact and belongs here, because it
is the sidecar's decision:

| Condition, evaluated in order | `EDGE_STATUS` |
|---|---|
| No relay connection | `open: false`, `reason: "peer_unreachable"` |
| No slot granted, or `ringSize` is 1 | `open: false`, `reason: "no_peer"` |
| East neighbour `live: false` | `open: false`, `reason: "no_peer"` |
| East neighbour `modConnected: false` | `open: false`, `reason: "peer_mod_absent"` |
| East neighbour `gameVersion` incompatible | `open: false`, `reason: "peer_incompatible"` |
| East neighbour `simulationSize` unequal by Contract A §13 A10 | `open: false`, `reason: "sim_size_mismatch"` |
| Operator closed the edge | `open: false`, `reason: "admin_closed"` |
| Otherwise | `open: true`, `reason: "peer_live"`, `peerSimulationSize` = the neighbour's `S` |

**The ripple is one-directional, like the lanes.** When a peer dies:

- its **west** neighbour loses its export target and closes its export edge;
- its **east** neighbour is unaffected and is told nothing — it simply receives nothing;
- the dead peer's own slot stays in the ring, reserved, `live: false`.

A sidecar that loses its **mod** must also publish that: it reports
`modConnected: false` on its next `SECTOR_CLAIM`, and the relay's `PEER_STATUS` carries it
to the west neighbour, which closes its export edge with `peer_mod_absent`. A dead sim must
not keep receiving organisms (Contract A §8, step 2).

---

## 9. The custody chain, and bounce-back

Contract A owns steps 1, 2, 4 and 5 of `system_decomposition.md`'s chain. Contract B owns
the middle, and the diagram is unchanged from M2 except for the copy to the archive:

```
mod A            sidecar A                relay             sidecar B            mod B
  │ MIGRATE_OUT      │                      │                   │                  │
  ├─────────────────►│ hash + journal+fsync │                   │                  │
  │ MIGRATE_OUT_ACK  │                      │                   │                  │
  │◄─────────────────┤                      │                   │                  │
  │  (destroys)      │ MIGRATION_PAYLOAD    │                   │                  │
  │                  ├─────────────────────►├──────────────────►│ journal + fsync  │
  │                  │                      ├╌╌► archive (copy) │ MIGRATE_IN       │
  │                  │                      │                   ├─────────────────►│
  │                  │                      │                   │ MIGRATE_IN_ACK   │
  │                  │                      │                   │◄─────────────────┤
  │                  │  MIGRATION_ACK       │                   │                  │
  │                  │◄─────────────────────┤◄──────────────────┤ (tombstone)      │
  │                  │ (tombstone)          ├╌╌► archive (copy) │                  │
```

Every hop deduplicates on `migrationId`, so any frame in this diagram can be replayed
safely. The dotted copies are best-effort and hold nothing up (§5.1).

**Bounce-back, and the one rule that keeps it safe.** Custody chain step 6 says a remote
NACK or a timeout re-injects the organism into the origin sim. Taken literally, a
**timeout** can duplicate an organism: the destination may have delivered and ACKed, and
only the ACK may have been lost. D2 forbids duplication and prefers loss, so the rule is
narrowed — unchanged from M2, and now also `contract-a.md` §13, A6:

| Situation | Origin sidecar's action |
|---|---|
| `MIGRATION_NACK` received, any code | **Bounce.** A NACK is only ever sent before durable custody (§6.8), so it proves the organism is not at the destination. |
| The forward never reached a live peer — the relay link is down, or `destSlot` is vacant — for longer than `bounceTimeoutMs` | **Bounce.** The frame was never handed to anyone. |
| The forward reached a live peer and no answer came back | **Hold and re-forward.** Never bounce. The destination deduplicates, so a re-forward is free, and holding converts a possible duplication into a bounded delay. |

A bounce re-delivers the organism to the origin's own mod as a Contract A `MIGRATE_IN` with
`bounceBack: true`, **on the edge it left from** — which under the ring is the origin's own
`exportEdge`, not its passive entry edge. The organism therefore lands inside its own
capture band, moving outward, and only the entry-immunity window stops it re-exporting on
the next tick; `contract-a.md` §14 A11 makes that window REQUIRED for exactly this reason.

The mirror-image rule protects the destination. When the destination mod answers
`MIGRATE_IN_NACK` with a **permanent** code after the payload was already journaled, the
destination sidecar **holds** the entry in its journal for an operator and logs it loudly.
It does not return the organism over Contract B, because there is no two-phase return in
M3's message list and a lost return frame would lose the organism outright. Contract A §5.9
and §13 A7 say the same thing from the other side.

---

## 10. The archive: genomes, gaps and rate limits

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
characters of the digest. It survives a restart — Risk 7's fourth failure is a sidecar
restart losing its cache, and an in-memory cache would guarantee it. Entries expire after
`genomeCacheRetentionDays` (30) with a least-recently-served policy, bounded by
`genomeCacheMaxBytes` (2 GiB).

**Fetch behaviour, archive side.**

| Rule | Statement |
|---|---|
| Never block | A migration record is written when the envelope arrives. A missing genome is a **gap** on that record, never a reason to delay or refuse a record. |
| Ask the source first | The envelope's `sourcePeer` hashed the blob and cached it, so it is the peer most likely to have it. |
| Then ask around | On `unknown_hash`, try other live peers in ring order, one at a time. |
| Rate limit | At most `genomeRequestsPerMinute` (30) requests from one requester to one peer. The answering sidecar enforces the same limit and answers `rate_limited` above it. |
| Retry schedule | 1 minute, 5 minutes, 30 minutes, 6 hours, then daily. Reset the ladder when the ring's membership changes — a peer that just came back may hold what nobody had. |
| Keep the hash forever | A hash with no genome is a permanent, useful record: it is still a lineage-graph node, and a fetch that failed for a year can succeed tomorrow. |
| Verify | Recompute the hash of every fetched blob (§6.10). |
| Report | The gap report lists every hash no peer has served, with its first-seen time and attempt count. It is the archive's honest statement of what it does not have. |

**The known limit, restated because it is load-bearing** (D11): a database built from
migrations holds migrants and their ancestors, never the resident population of a peer.
Census uploads would close the gap and are not M3.

---

## 11. Deviations, and why

1. **`SECTOR_CLAIM` and `SECTOR_GRANT` keep their names.** The unit they carry is a ring
   slot, not a sector. `system_decomposition.md`'s ratified Contract B list still names
   both messages, with ring semantics attached, and renaming a ratified message to fix a
   noun would put this document and the decomposition out of step for no wire benefit. The
   **fields** are renamed throughout — `slot`, `preferredSlot`, `sourceSlot`, `destSlot` —
   so nothing inside a frame still says "sector".
2. **JSON, not Protobuf.** Contract B's format was left open ("Protobuf likely"). M3 keeps
   the JSON envelope of Contract A so one codec, one test harness and one set of eyes cover
   both wires. The `bb8` body is still an opaque string, so the D4 boundary is unchanged and
   a later Protobuf framing changes no message shape.
3. **`entityId` and `heading` are carried explicitly** (§6.6), even though Contract C's
   field list has neither. Contract A's `MIGRATE_IN` requires both. **Which value wins is
   not the same for the two fields, and the asymmetry is deliberate** — carried unchanged
   from `contract-b-m2.md` amendments B1 and B2:

   | Field | Authority | Why |
   |---|---|---|
   | `entityId` | The **blob**, when `bb8-schema` can read one. A wire value that disagrees is overridden and logged as a warning. | The blob is what `LoadBibiteOrEggFromData` restores, so the blob's id is the id that will actually exist. The mod's durable dedup key (Contract A §7.3) has to match it. |
   | `heading` | The **wire**. The blob's `$.rb2d.r` fills in only when the wire field is absent or zero. | The receiving mod rewrites `$.transform.rotation` and `$.rb2d.r` from the `MIGRATE_IN` heading before it restores (Contract A §5.7 step 3), so the wire value is authoritative by construction. |

   **The shape of `$.body.id` is not fixed by Contract A.** Its §5.3 example elides the
   `BibiteID` wrapper — `"body":{"id":-843827577, ... }` — while the game's own type is
   `BibiteID` with an inner `id`. `bb8-schema` **MUST** accept both: `{"body":{"id":N}}` and
   `{"body":{"id":{"id":N}}}`. Extraction is best-effort throughout — every field `Inspect`
   reads is optional, and a blob it cannot parse falls back to the wire values.
4. **`HANDSHAKE_ACK`, `SECTOR_GRANT` and `PEER_STATUS` are not in the ratified message
   list.** It names only the requests. A relay-arbitrated ring needs a reply and a liveness
   broadcast; these three are the smallest set that provides them. Carried from M2.
5. **The relay's ring memory is durable, and M2's was not.** M2 listed the RAM-only sector
   map as an open item for a later milestone. A reservation that never expires (D8) makes
   that a correctness requirement rather than a convenience, so `ring.json` lands in M3
   (§7.4).
6. **The relay gained one rule that is not frame forwarding**: the subscriber fan-out
   (§5.1). D1 keeps the relay dumb, and this rule keeps it dumb in the sense that matters —
   it still never parses a body, never validates a payload and never indexes anything. §5.1
   records the alternative and the cost of reversing the choice.
7. **`entryEdge` is still not on the wire.** The receiving sidecar derives it. Under the
   ring it is `"W"` for ordinary traffic and the receiver's own `exportEdge` for a
   bounce-back, and both are facts the receiver already knows about itself. Sending it would
   let a sender dictate a receiver's geometry.

---

## 12. Tunables and defaults

| Name | Default | Owner | Meaning |
|---|---|---|---|
| `relayPort` | `8790` | both | The relay's listen port. Opened in the Windows Firewall on the relay's machine. |
| `relayPingIntervalMs` | `5000` | relay | Application-level `PING` cadence. |
| `peerTimeoutMs` | `15000` | relay | Silence before a peer is dropped and its slot goes `live: false`. |
| `relayBackoffMinMs` | `1000` | client | Reconnect floor. |
| `relayBackoffMaxMs` | `30000` | client | Reconnect ceiling. |
| `stableSessionMs` | `5000` | client | How long a connection must live before the backoff ladder resets (Contract A §13, A8). |
| `authFailuresBeforeCeiling` | `5` | client | Consecutive HTTP 401s before the backoff is pinned at the ceiling (§3.1). |
| `forwardRetryMs` | `5000` | sidecar | Re-forward cadence for a journaled outbound entry with no answer. |
| `bounceTimeoutMs` | `20000` | sidecar | How long an outbound entry that never reached a live peer waits before it is bounced home. |
| `migrationAckTimeoutMs` | `30000` | sidecar | Informational deadline for `MIGRATION_ACK`; expiry re-forwards, it never bounces (§9). |
| `inboundQueueMax` | `64` | sidecar | Shared with Contract A §10. |
| `exportRetentionSeconds` | `3600` | sidecar | Tombstone lifetime. Shared with Contract A §10. |
| `maxFrameBytes` | `8388608` | both | Shared with Contract A §10. |
| `maxPayloadBytes` | `4194304` | both | Shared with Contract A §10. Applies to `body.bb8` and to a `GENOME_RESPONSE` body. |
| `archiveQueueMax` | `1024` | relay | Copied frames buffered per subscriber before the oldest is dropped (§5.1). |
| `genomeRequestTimeoutMs` | `15000` | requester | How long a requester waits for a `GENOME_RESPONSE` before it counts the attempt as failed. |
| `genomeRequestsPerMinute` | `30` | both | Per requester, per answering peer. Enforced on both sides (§10). |
| `genomeCacheRetentionDays` | `30` | sidecar | Genome cache lifetime, least-recently-served. |
| `genomeCacheMaxBytes` | `2147483648` | sidecar | 2 GiB cap on `<data-dir>/genomes/`. |

---

## 13. Open items for M4

1. **No TLS, and one shared token** (§3.1). The wire is plain HTTP on a LAN, so a genome, a
   peer id and the token itself are readable in transit, and any token holder can present
   any `peerId`. M4 brings TLS and per-peer credentials together, because splitting them
   produces a half-secured relay that reads as secured.
2. **A permanently rejected inbound organism is held, never returned** (§9). A safe
   two-phase return needs one more message pair. It stays parked while "held for an
   operator" remains an honest answer.
3. **Ring insertion under churn is untested by design.** M3 inserts three known peers, once
   each, by hand. Strangers joining and leaving continuously is M4's problem, and it is
   where the append-at-tail rule will first be stressed.
4. **The archive has no write interface and no authentication of its own.** It is a
   subscriber that trusts the ring token. A public relay cannot copy every envelope to
   whoever asks, so M4 needs a subscriber authorisation rule.
5. **Slot release is a startup flag** (§7.5). If the ring ever grows past what one operator
   can restart at will, release needs an authenticated admin path — which is another reason
   it waits for the milestone that brings authentication.
