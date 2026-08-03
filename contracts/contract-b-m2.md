# Contract B — M2 Subset (Sidecar ↔ Relay ↔ Sidecar)

**Version:** `contract-b/1`
**Amended:** 2026-08-02. Three resolutions from the Go implementation are folded into §8 —
**B1** the authority order for `entityId` and `heading` (item 2), **B2** the two accepted
shapes of `$.body.id` (item 2), **B3** the on-disk sector file (item 4). Contract A's
matching amendment set is `contract-a.md` §13; A6 and A7 there align Contract A's wording
with §7 of this document, and change nothing here.
**Status:** the minimum M2 needs. Not the full Contract B.

`system_decomposition.md` lists ten Contract B message types. M2 needs six of them plus
three relay-control replies, on one machine, over loopback, between exactly two sidecars.
`TOPOLOGY_GOSSIP`, `PEER_EXCHANGE`, `CATALOG_QUERY` and `CATALOG_RESPONSE` are **out of
scope** — they arrive with M4 and M5. Transport security, peer authentication, and more
than two sectors are also out of scope (M3/M4).

This document is written in the spirit of `contracts/contract-a.md` and reuses its
envelope, its version rules, and its RFC 2119 key words. Where Contract A already answers
a question, this document points at it instead of restating it.

---

## 1. Topology

M2 has exactly two sectors, `A` and `B`, side by side:

```
        ┌──────────┐ ┌──────────┐
        │ sector A │ │ sector B │
        │        E │ │ W        │
        └──────────┘ └──────────┘
             peer-a       peer-b
                 ╲         ╱
                  ╲       ╱
                ┌───────────┐
                │   relay   │
                └───────────┘
```

- The **east** edge of `A` pairs with the **west** edge of `B`.
- An organism that exits `A` on `E` enters `B` on `W`, and the reverse. The receiving
  sidecar computes `entryEdge` with Contract A §4.2's opposite-edge function and copies
  `exitPosition` into `entryPosition` unchanged, because the M2 sectors are pure
  translations of one another (Contract A §4.4).
- The relay is the single arbiter of the sector map (D1). No sidecar chooses its own
  sector.

---

## 2. Transport

| Property | Value |
|---|---|
| Protocol | WebSocket (RFC 6455) over plain HTTP. TLS is M3. |
| URL | `ws://{relay-host}:{port}/contract-b/v1` |
| Default port | `8790` |
| Roles | The **relay is the server**. Every sidecar is a **client** and does all the dialling. |
| Frame type | Text frames. One JSON object per frame. No batching. |
| Encoding | UTF-8, no BOM |
| Compression | `permessage-deflate` **MUST NOT** be negotiated |
| Max frame size | 8 MiB (`maxFrameBytes`), same as Contract A |
| Reconnect | The sidecar reconnects with exponential backoff and full jitter, 1000 ms to 30000 ms — the same rule Contract A §6.2 gives the mod. |

### 2.1 Close codes

| Code | Name | Sent by | Meaning |
|---|---|---|---|
| `1000` | `NORMAL` | either | Clean shutdown. |
| `1009` | `TOO_BIG` | either | Frame over `maxFrameBytes`. |
| `4000` | `PROTOCOL_UNSUPPORTED` | relay | The `protocol` major version is not supported. The sidecar **MUST NOT** reconnect until it is restarted. |
| `4003` | `MALFORMED_FRAME` | either | Not valid JSON, a missing REQUIRED envelope field, no `HANDSHAKE` first, or a routing field that disagrees with the sender's own peer id. Reconnect with backoff. |
| `4004` | `LIVENESS_TIMEOUT` | relay | No frame and no `PONG` within `peerTimeoutMs`. Reconnect with backoff. |
| `4005` | `SHUTTING_DOWN` | either | The sender is draining. Reconnect with backoff. |
| `4006` | `REPLACED` | relay | A newer connection claimed the same `peerId`. The old connection **MUST NOT** reconnect. |

---

## 3. The envelope

Identical in shape to Contract A §3 — five fields, no more:

```json
{
  "protocol": "contract-b/1",
  "type": "MIGRATION_PAYLOAD",
  "messageId": "b7d1e0c4-9f2a-4c31-8b6d-2e0a41f5c7a9",
  "sentAt": 1785693600123,
  "data": { }
}
```

The rules of Contract A §3.1 and §3.2 apply unchanged: compatibility is on the **major**
version, changes within a major version are additive fields only, unknown fields inside
`data` and inside the envelope are ignored, an unknown `type` is ignored with one warning
and **never** closes the connection, and a malformed frame closes with `4003`.

Timestamps are informational (D5). `messageId` is for log correlation only; `migrationId`
is the one idempotency key in the system (Contract A §7.1).

---

## 4. Routing, and what the relay may read

The relay reads exactly two fields out of `data` and nothing else:

| Field | Present on | Routes to |
|---|---|---|
| `destSector` | `MIGRATION_PAYLOAD` | The live peer holding that sector |
| `destPeer` | `MIGRATION_ACK`, `MIGRATION_NACK` | That peer id |

Every routable frame also carries `sourcePeer`. The relay **MUST** check it against the
sending connection's own peer id and **MUST** close with `4003` when they disagree. It
**MUST NOT** rewrite it, so the frame can be forwarded byte for byte.

The relay **MUST NOT** decode `data.body.bb8`, and **MUST NOT** validate a payload
(D4 — that is `bb8-schema`'s job, sidecar-side, at both ends). It forwards the original
frame bytes unchanged.

When the destination sector is vacant or the destination peer is not connected, the relay
**MUST** answer the *sender* with `MIGRATION_NACK` (`SECTOR_VACANT` or `PEER_UNKNOWN`)
rather than drop the frame. A dropped frame turns a bounded failure into a stall.

---

## 5. Message catalogue

Nine types.

| Type | Direction | Answered by |
|---|---|---|
| `HANDSHAKE` | sidecar → relay | `HANDSHAKE_ACK`, or a close |
| `HANDSHAKE_ACK` | relay → sidecar | nothing |
| `SECTOR_CLAIM` | sidecar → relay | `SECTOR_GRANT` |
| `SECTOR_GRANT` | relay → sidecar | nothing |
| `PEER_STATUS` | relay → sidecar | nothing |
| `MIGRATION_PAYLOAD` | sidecar → sidecar, forwarded | `MIGRATION_ACK` or `MIGRATION_NACK` |
| `MIGRATION_ACK` | sidecar → sidecar, forwarded | nothing |
| `MIGRATION_NACK` | sidecar → sidecar or relay → sidecar | nothing |
| `PING` / `PONG` | either | `PONG` / nothing |

### 5.1 `HANDSHAKE` — sidecar → relay

The **first frame on every connection**. Any other first frame closes with `4003`.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `peerId` | string | yes | Stable identity of this sidecar, from `--peer-id`. It is what makes a sector reclaim work across a restart. |
| `protocolVersion` | string | yes | `"contract-b/1"`. A different major closes with `4000`. |
| `gameVersion` | string | yes | The game version behind this sidecar, from the mod's `CONFIG_UPDATE`. Empty while no mod is connected. |
| `sidecarVersion` | string | yes | Informational. |
| `simulationSize` | float | no | `S`, when a mod has already reported one. |

A second connection presenting a `peerId` that is already live **MUST** cause the relay to
close the older connection with `4006` and serve the newer one — the same self-healing
rule Contract A §2 gives the mod socket.

### 5.2 `HANDSHAKE_ACK` — relay → sidecar

| Field | Type | Required | Semantics |
|---|---|---|---|
| `relayVersion` | string | yes | Informational. |
| `protocolVersion` | string | yes | `"contract-b/1"`. |
| `assignedSector` | string | no | The sector this `peerId` already holds, when the relay remembers one. Absent otherwise. |

### 5.3 `SECTOR_CLAIM` — sidecar → relay

Sent right after `HANDSHAKE`, and again whenever `simulationSize` or `borderEdges`
change. A repeat claim for a sector this peer already holds is an **update**, not a new
claim.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `preferredSector` | string | no | `"A"` or `"B"`. Advisory. A sidecar that has held a sector before persists it and asks for it again, so a relay restart does not swap the two sims. §8 item 4 names the file it persists it in. |
| `simulationSize` | float | yes | `S`, as last reported by the mod. `0` while no mod is connected. |
| `borderEdges` | array of edge | yes | The mod's declared edges. Empty while no mod is connected. |
| `gameVersion` | string | no | Updates the value from `HANDSHAKE`. |

**Arbitration.** The relay assigns from the fixed set `["A", "B"]`:

1. If this `peerId` already holds a sector, it keeps it. This is the reclaim-on-reconnect
   rule, and it is keyed on `peerId`, never on the connection.
2. Otherwise, if `preferredSector` is free, it is granted.
3. Otherwise the first free sector in `["A", "B"]` is granted — so the first peer gets
   `A` and the second gets `B`.
4. Otherwise the claim is refused with `granted: false`, `reason: "no_sector_available"`.
   M2 is two sectors; a third peer is a mis-wired rig.

### 5.4 `SECTOR_GRANT` — relay → sidecar

| Field | Type | Required | Semantics |
|---|---|---|---|
| `granted` | bool | yes | |
| `sector` | string | no | Present when `granted` is `true`. |
| `reason` | string enum | yes | `"granted"`, `"reclaimed"`, `"no_sector_available"`, `"protocol_mismatch"`. |

### 5.5 `PEER_STATUS` — relay → sidecar

**Full state, not a delta**, exactly like Contract A's `EDGE_STATUS`. Sent after every
registry change: a peer connecting, a peer dying, a sector granted, a `simulationSize`
update.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `epoch` | number (int64) | yes | Strictly increasing per connection, from 1. A receiver **MUST** ignore an epoch lower than or equal to the last applied one. Resets on a new connection. |
| `peers` | array of object | yes | One entry per sector that has a **live** peer. |
| `peers[].peerId` | string | yes | |
| `peers[].sector` | string | yes | `"A"` or `"B"`. |
| `peers[].gameVersion` | string | yes | |
| `peers[].simulationSize` | float | yes | The peer's `S`. |
| `peers[].modConnected` | bool | yes | Whether that sidecar currently has a live mod. A sidecar with no mod cannot receive organisms. |

**A sector absent from `peers` is vacant.** That is the whole liveness mechanism: when a
peer's connection drops, the relay removes it, bumps the epoch, and broadcasts a
`PEER_STATUS` in which its sector is missing. The surviving sidecar sees the vacancy,
closes the paired edge, and pushes a new Contract A `EDGE_STATUS` to its own mod with
`open: false` and `reason: "no_peer"`. When the *relay itself* is unreachable, the sidecar
closes every edge with `reason: "peer_unreachable"` — it cannot know anything about its
neighbour while the link is down.

### 5.6 `MIGRATION_PAYLOAD` — sidecar → sidecar, forwarded

The Contract C `MigrationEnvelope`, carried in `data`.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The idempotency key (D2). Minted by the origin **mod**, preserved end to end. |
| `kind` | string enum | yes | `"bibite"` in M2. Anything else is answered `MIGRATION_NACK` / `KIND_UNSUPPORTED`. |
| `body.version` | string | yes | The game version that serialized the blob. Authoritative over the blob's own `version` key (Contract A §4.6). |
| `body.bb8` | string | yes | The opaque blob, as a JSON **string**, never nested, never base64. Max `maxPayloadBytes` (4 MiB). |
| `sourcePeer` | string | yes | Origin peer id. The relay verifies it. |
| `sourceSector` | string | yes | |
| `destSector` | string | yes | The relay routes on this. |
| `exitEdge` | edge enum | yes | The edge of the **source** sim (Contract A §4.2). |
| `exitPosition` | float | yes | `[0,1]` along that edge, by Contract A §4.3. Already clamped by the origin mod. |
| `velocity` | `{x,y}` | yes | Copied, never mirrored (Contract A §4.4). |
| `timestamp` | `timestampMs` | yes | Informational (D5). |
| `entityId` | `entityId` | yes | Signed int32, often negative. **M2 addition** — see §8. |
| `heading` | float | yes | Degrees (Contract A §4.4). **M2 addition** — see §8. |

The receiving sidecar **MUST**, in this order:

1. Deduplicate on `migrationId` against its journal **and its tombstones**. A hit is
   answered `MIGRATION_ACK` immediately and delivered nothing (Contract A §7.2).
2. Apply admission control. More than `inboundQueueMax` (64) un-delivered journal entries
   is answered `MIGRATION_NACK` / `OVERLOADED`.
3. Validate the blob with `bb8-schema` against `body.version`. Failure is
   `MIGRATION_NACK` / `INVALID_PAYLOAD`.
4. Write the journal entry and **flush it to durable storage**. Custody moves here.
5. Deliver it to its mod as Contract A `MIGRATE_IN`, replaying until the mod ACKs
   (Contract A §7.5).

### 5.7 `MIGRATION_ACK` — sidecar → sidecar, forwarded

**Sent only after the receiving mod's Contract A `MIGRATE_IN_ACK`.** This is the link in
the custody chain that lets the origin sidecar drop its own journal entry (D2, custody
chain step 5). A sidecar **MUST NOT** send `MIGRATION_ACK` when it has merely journaled
the payload.

The one exception is the deduplication path of §5.6 step 1: a `migrationId` that is
already tombstoned was ACKed once before, so re-ACKing it carries the same meaning.

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
delivery.

### 5.8 `MIGRATION_NACK` — sidecar → sidecar, or relay → sidecar

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
| `SECTOR_VACANT` | transient | relay | No live peer holds `destSector`. |
| `PEER_UNKNOWN` | transient | relay | `destPeer` is not connected. |
| `OVERLOADED` | transient | sidecar | `inboundQueueMax` reached. |
| `SIM_SIZE_MISMATCH` | transient | sidecar | The two sims disagree about `S`. |
| `MOD_ABSENT` | transient | sidecar | No mod is connected and the queue is full. |
| `INVALID_PAYLOAD` | permanent | sidecar | `bb8-schema` rejected the blob, or it is over `maxPayloadBytes`. |
| `KIND_UNSUPPORTED` | permanent | sidecar | `kind` is not `"bibite"`. |
| `VERSION_UNSUPPORTED` | permanent | sidecar | No `bb8-schema` dialect for `body.version`. |
| `MALFORMED_MESSAGE` | permanent | sidecar | A `data` field failed validation. |
| `SHUTTING_DOWN` | transient | either | The sender is draining. |

**A sidecar MUST NOT send `MIGRATION_NACK` after it has durably journaled the payload.**
This is what makes a NACK a *definitive* statement that custody never moved, and it is
what makes the origin's bounce-back safe. See §7.

### 5.9 `PING` / `PONG`

`data` is `{"nonce": "<uuid>"}` on `PING` and the same nonce on `PONG`. Either side may
ping. The relay pings every `relayPingIntervalMs` and closes a peer with `4004` after
`peerTimeoutMs` of complete silence. WebSocket-level ping/pong frames are used as well;
the application-level pair exists so a sidecar can measure the relay path itself.

---

## 6. The custody chain across Contract B

Contract A owns steps 1, 2, 4 and 5 of `system_decomposition.md`'s chain. Contract B owns
the middle:

```
mod A            sidecar A                relay             sidecar B            mod B
  │ MIGRATE_OUT      │                      │                   │                  │
  ├─────────────────►│ journal + fsync      │                   │                  │
  │ MIGRATE_OUT_ACK  │                      │                   │                  │
  │◄─────────────────┤                      │                   │                  │
  │  (destroys)      │ MIGRATION_PAYLOAD    │                   │                  │
  │                  ├─────────────────────►├──────────────────►│ journal + fsync  │
  │                  │                      │                   │ MIGRATE_IN       │
  │                  │                      │                   ├─────────────────►│
  │                  │                      │                   │ MIGRATE_IN_ACK   │
  │                  │                      │                   │◄─────────────────┤
  │                  │  MIGRATION_ACK       │                   │                  │
  │                  │◄─────────────────────┤◄──────────────────┤ (tombstone)      │
  │                  │ (tombstone)          │                   │                  │
```

Every hop deduplicates on `migrationId`, so any frame in this diagram can be replayed
safely.

---

## 7. Bounce-back, and the one rule that keeps it safe

Custody chain step 6 says a remote NACK or a timeout re-injects the organism into the
origin sim. Taken literally, a **timeout** can duplicate an organism: the destination may
have delivered and ACKed, and only the ACK may have been lost. D2 forbids duplication and
prefers loss, so M2 narrows the rule:

| Situation | Origin sidecar's action |
|---|---|
| `MIGRATION_NACK` received, any code | **Bounce.** A NACK is only ever sent before durable custody (§5.8), so it proves the organism is not at the destination. |
| The forward never reached a live peer — the relay link is down, or the destination sector is vacant — for longer than `bounceTimeoutMs` | **Bounce.** The frame was never handed to anyone. |
| The forward reached a live peer and no answer came back | **Hold and re-forward.** Never bounce. The destination deduplicates, so a re-forward is free, and holding converts a possible duplication into a bounded delay. |

A bounce re-delivers the organism to the origin's own mod as a Contract A `MIGRATE_IN`
with `bounceBack: true`, on the edge it left from, and is journaled and ACKed like any
other inbound delivery.

The mirror-image rule protects the destination. When the destination mod answers
`MIGRATE_IN_NACK` with a **permanent** code after the payload was already journaled,
the destination sidecar **holds** the entry in its journal for an operator and logs it
loudly. It does not return it over Contract B, because there is no two-phase return in
M2's message list and a lost return frame would lose the organism outright. Contract A
§5.9 explicitly permits this choice.

---

## 8. Deviations from `system_decomposition.md`, and why

1. **JSON, not Protobuf.** Contract B's format was left open ("Protobuf likely"). M2 uses
   the same JSON envelope as Contract A so one codec, one test harness and one set of
   eyes cover both wires. The `bb8` body is still an opaque string, so the D4 boundary is
   unchanged and a later Protobuf framing changes no message shape.
2. **`entityId` and `heading` are carried explicitly** (§5.6), even though Contract C's
   field list has neither. Contract A's `MIGRATE_IN` requires both. Contract A §5.7 says
   `entityId` is "extracted from the blob by `bb8-schema`", and §4.4 puts `heading` at
   `$.rb2d.r` — both are still true. Carrying them explicitly keeps the delivery path
   working while `bb8-schema` is still a skeleton, and it is an additive change.

   **Which value wins is not the same for the two fields, and the asymmetry is
   deliberate** (amendment B1):

   | Field | Authority | Why |
   |---|---|---|
   | `entityId` | The **blob**, when `bb8-schema` can read one. A wire value that disagrees is overridden and logged as a warning. | The blob is what `LoadBibiteOrEggFromData` restores, so the blob's id is the id that will actually exist. The mod's durable dedup key (Contract A §7.3) has to match it. |
   | `heading` | The **wire**. The blob's `$.rb2d.r` fills in only when the wire field is absent or zero. | The receiving mod rewrites `$.transform.rotation` and `$.rb2d.r` from the `MIGRATE_IN` heading before it restores (Contract A §5.7 step 3), so the wire value is authoritative by construction. |

   **The shape of `$.body.id` is not fixed by Contract A** (amendment B2). Its §5.3 example
   elides the `BibiteID` wrapper — `"body":{"id":-843827577, ... }` — while the game's own
   type is `BibiteID` with an inner `id`. The two are indistinguishable from that document
   alone, so `bb8-schema` **MUST** accept both: `{"body":{"id":N}}` with a bare number, and
   `{"body":{"id":{"id":N}}}` with the wrapper. Neither is an error. Extraction is
   best-effort throughout — every field `Inspect` reads is optional, and a blob it cannot
   parse simply falls back to the wire values.
3. **`HANDSHAKE_ACK`, `SECTOR_GRANT` and `PEER_STATUS` are new names.** The ratified list
   names only the requests. A relay-arbitrated sector map needs a reply and a liveness
   broadcast; these three are the smallest set that provides them.
4. **`preferredSector` on `SECTOR_CLAIM`** (§5.3) exists so a relay restart cannot swap
   the two sims. The sidecar persists its granted sector in `--data-dir` and asks for it
   again. The file is `<data-dir>/sector`, one line, `A` or `B` (amendment B3). It is
   written on every **granted** `SECTOR_GRANT` and read once at startup. An explicit
   `--sector` flag overrides it. An unreadable or unrecognised value is treated as absent,
   which leaves the claim to §5.3's arbitration rules 3 and 4. It is deliberately **not**
   in the journal: losing it costs a sector swap, not an organism.
5. **The timeout half of bounce-back is narrowed** (§7). This is a correctness fix, not a
   convenience: the literal rule duplicates organisms, which D2 rules out.

---

## 9. Tunables and defaults

| Name | Default | Owner | Meaning |
|---|---|---|---|
| `relayPort` | `8790` | both | The relay's listen port. |
| `relayPingIntervalMs` | `5000` | relay | Application-level `PING` cadence. |
| `peerTimeoutMs` | `15000` | relay | Silence before a peer is dropped and its sectors go vacant. |
| `relayBackoffMinMs` | `1000` | sidecar | Reconnect floor. |
| `relayBackoffMaxMs` | `30000` | sidecar | Reconnect ceiling. |
| `forwardRetryMs` | `5000` | sidecar | Re-forward cadence for a journaled outbound entry with no answer. |
| `bounceTimeoutMs` | `20000` | sidecar | How long an outbound entry that never reached a live peer waits before it is bounced home. |
| `migrationAckTimeoutMs` | `30000` | sidecar | Informational deadline for `MIGRATION_ACK`; expiry re-forwards, it never bounces (§7). |
| `inboundQueueMax` | `64` | sidecar | Shared with Contract A §10. |
| `exportRetentionSeconds` | `3600` | sidecar | Tombstone lifetime. Shared with Contract A §10. |
| `maxFrameBytes` | `8388608` | both | Shared with Contract A §10. |
| `maxPayloadBytes` | `4194304` | both | Shared with Contract A §10. |

---

## 10. Open items for M3

1. **No authentication and no TLS.** Any local process can claim any `peerId` and take a
   sector. M2 binds loopback. A shared token on the WebSocket upgrade is the M3 addition,
   the same one Contract A §12 proposes.
2. **The relay's sector memory is in RAM.** A relay restart forgets which peer held which
   sector; `preferredSector` covers the two-sector case, but a real deployment wants a
   durable, leased sector map (M4).
3. **Two sectors only.** The `["A", "B"]` set, the fixed east/west pairing, and the
   opposite-edge mapping are hard-coded. M3 generalises to an `{x, y}` grid.
4. **A permanently rejected inbound organism is held, never returned** (§7). A safe
   two-phase return needs one more message, and it belongs with M3's journal-recovery
   hardening.
