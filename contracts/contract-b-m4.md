# Contract B — M4 (Sidecar ↔ Relay ↔ Sidecar ↔ Archive)

**Version:** `contract-b/3.4`
**Amended:** 2026-08-05, from the Go implementation (commit `823a70f`). Four resolutions are
folded into the body and recorded in **§14** — **B4** the missing `statsBroadcastIntervalMs`
default (§6.5, §12), **B5** the retry a held entry must keep running (§9.2, §9.3), **B6** the
narrowing of the duplicate re-ACK to a tombstone (§6.6, §6.7), **B7** the fan-out of the
relay's own non-delivery answers and the NACK dedup key (§5.1). All four are clarifying; the
version does not move. Contract A's matching set is `contract-a.md` §15, A26 and A27.
**Amended:** 2026-08-07, amendment set `contract-b/3.1 + B9–B10` (**§15**), from the owner's
ratification of **Option A — species identity travels in the migration envelope**.
`MIGRATION_PAYLOAD` gains one OPTIONAL `species` object, carried opaquely end to end and
recorded by the archive; the relay and both sidecars validate its shape and interpret nothing.
That is an additive field, so §4's own test answers with a **minor** bump to `contract-b/3.1`.
Contract A's matching set is `contract-a.md` §16, A30–A33. Affected body text carries an
`(amended — §15, Bx)` or `(added — §15, Bx)` marker, and **§15 wins over the body and over
§14 wherever they disagree.**
**Amended:** 2026-08-07, amendment set `contract-b/3.2 + B11–B12` (**§16**), from the owner's
ratification of **the species census on the live map**. The **peer stats block** (§6.3.1)
gains the same `species` array `contract-a.md` §17 puts on `HEARTBEAT`, copied verbatim from
the last heartbeat and republished blind in `PEER_STATUS`; and §10.1's rule that the archive's
species names are not a page input is **amended**, because the page now renders a species view
— from the census in `PEER_STATUS`, never from the migration ledger. That is an additive
field, so §4's own test answers with a **minor** bump to `contract-b/3.2`. Contract A's
matching set is `contract-a.md` §17, A35–A37. Affected body text carries an
`(amended — §16, Bx)` or `(added — §16, Bx)` marker, and **§16 wins over the body and over
§14 and §15 wherever they disagree.**
**Amended:** 2026-08-07, amendment set `contract-b/3.3 + B13–B15` (**§17**), from the owner's
ratification of **D17 two-way lanes, D19 the live hop animation and D20 the pacing raise**.
Every lane becomes bidirectional: `SECTOR_GRANT.neighbours` gains the `"W"` and `"S"` keys and
§8's walk runs in both directions per axis, which is where the real work of a two-way map
lives — `contract-a.md` §18 takes **no** version bump, because its fields have accepted four
edges since A18. Two new keys in an existing enum-keyed map are additive, so §4's own test
answers with a **minor** bump to `contract-b/3.3`. Contract A's matching set is
`contract-a.md` §18, A38–A41. Affected body text carries an `(amended — §17, Bx)` or
`(added — §17, Bx)` marker, and **§17 wins over the body and over §14, §15 and §16 wherever
they disagree.**
**Amended:** 2026-08-07, amendment set `contract-b/3.4 + B16–B17` (**§18**), from the owner's
ratification of **the pacing and speed readout on the live map**. The **peer stats block**
(§6.3.1) gains three OPTIONAL settings — `timeScale`, copied from the mod's `HEARTBEAT`, and
`inboundRatePerSimMinute` / `inboundRateBurst`, which are the sidecar's own configuration —
so the operator surface can say how fast each world runs and what cap its arrivals are queued
behind. `pacedDepth` has been on this block since M4 and has never been readable: a depth is
only deep against a cap, and that cap has moved three times. Three additive OPTIONAL fields,
so §4's own test answers with a **minor** bump to `contract-b/3.4`. `contract-a.md` takes
**no** bump — no field of that wire changes, and `HEARTBEAT.timeScale` has been mandatory
since `contract-a/2.0`. Affected body text carries an `(amended — §18, Bx)` or
`(added — §18, Bx)` marker, and **§18 wins over the body and over §14 to §17 wherever they
disagree.**
**Status:** implementation-ready for M4. Written 2026-08-05 from the ratified decisions
D12–D16 (`system_decomposition.md`), the amended D2, and the work order in
`m4_considerations.md`, *Contract Changes Needed*. Extended by D17–D20, ratified 2026-08-07
against the living deployment.
**Supersedes:** `contracts/contract-b-m3.md`, in full. That document is the historical
record of the M3 ring and is **not** current guidance.
**Companion documents:** `contracts/contract-a.md` (`contract-a/2.2`, mod ↔ sidecar) and
`contracts/genome-hash.md` (`bb8-genome/1`, the canonical genome projection — **unchanged by
M4, by the species block and by the census**, none of which is hashed, and whose one payload
key, `genes.speciesID`, that projection already excludes: §4.3 there).

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
   row 1:  ┌──◄► (0,1) ◄──► (1,1) ◄──► (2,1) ◄─┐
           └────────── east/west wrap ─────────┘
               ▲▼          ▲▼          ▲▼        north/south lanes wrap
               ││          ││          ││        between row 1 and row 0
   row 0:  ┌──◄► (0,0) ◄──► (1,0) ◄──► (2,0) ◄─┐
           └────────── east/west wrap ─────────┘

   The M4 target rig: a 3×2 map, six instances, two machines.
   Row 0 holds slots 1–3, row 1 holds slots 4–6.
   Under D17 every peer exports AND receives on all four edges.
   A lane still never returns to its own source: each walk visits every
   OTHER position on its axis and stops.
```

**Two-way lanes (D17, 2026-08-07).** Every lane above is bidirectional (amended — §17, B13).
A peer's `W` export routes to the first deliverable slot **west** along its row, which
receives it on its `E` edge; `S` routes down the column and is received on `N`. The four
lanes are independent and each closes on its own. **On an axis of length 2 the forward and
reverse walks name the same slot** — on this rig every column does, so slots 1 and 4 are a
two-lane pair. That is arithmetic, not a defect (§2.1).

| Term | Definition |
|---|---|
| **slot** | The **routing address** of a peer: an integer `≥ 1`. Never reused, never renumbered. `destSlot` names it. It replaces M2's sector and is unchanged from M3. |
| **position** | The **coordinate** `{col, row}`, both `≥ 0`. It decides who a peer's neighbours are, and nothing else. A position moves when the map grows; an address never moves. |
| **map** | The rectangle `{width, height}`: every `col` in `[0, width)` and every `row` in `[0, height)` is a position that exists. |
| **hole** | A position inside the rectangle with no reservation. A hole and a dark slot look identical to a router, which is why route-around is what makes a map with holes viable (D12). |
| **east lane** | From a position to the next **deliverable** slot east along its **row**, wrapping at the row's end. |
| **north lane** | From a position to the next **deliverable** slot north along its **column**, wrapping at the column's top. |
| **west lane**, **south lane** | The same two walks with the step **negated** (added — §17, B13). West runs along the row, south along the column, each wrapping at the other end. Nothing else about them differs: same deliverability filter, same skip list, same "visit every other position once and stop". |
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
- ~~**Out and in are different doors, per axis.** An organism that leaves through the east edge
  cannot come back through it; it must travel the whole row. The same holds for the column.
  There is no boomerang at a shared edge and no ping-pong to time out.~~
  **Superseded — §17, B13 (D17).** Every edge is now both doors. A round trip through one
  neighbour is legal and intended. **Nothing in this document depended on the retired
  property**, and that is worth checking rather than asserting: routing is on `destSlot`,
  custody is on `migrationId`, dedup is on `migrationId`, the hold clock runs on destination
  darkness, and `exitEdge` is carried for the record and read by no routing decision. The
  *instantaneous* no-re-export guarantee moves to `contract-a.md` §4.3.1 and §18 A38, where
  the geometry that enforces it lives.
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

**Under two-way lanes a degenerate axis behaves the same way and costs twice as much**
(added — §17, B13). Two facts compose, and both are arithmetic:

- **The forward and reverse walks name the same slot.** On an axis of length 2, one step
  forward and one step back are the same position mod 2. So a peer's `N` and `S` neighbours
  are one peer, and it exports to that peer on two lanes at once. On the 3×2 rig this is
  true of every column.
- **They therefore also close together.** Killing the column partner closes **both** `N` and
  `S` with `no_peer`. The table's row for `w×2` — *one hop is already a return trip* — now
  understates it: on a two-way axis of length 2 the pair is a shuttle, and an organism can
  cross and re-cross between exactly two worlds.

**The traffic consequence is the one to plan for.** A two-lane pair carries roughly twice the
hops of a one-way lane between the same two worlds, and it is the rig's *columns* that are
degenerate. An operator should expect column traffic to rise disproportionately against row
traffic on this map, and `contract-a.md` §18 A40 sizes the delivery rate limit with that
included. A **3×3** map is what makes both axes honest under two-way lanes, exactly as 3×2
made one axis honest under one-way ones.

---

## 3. Transport

| Property | Value |
|---|---|
| Protocol | WebSocket (RFC 6455) over plain HTTP. TLS is **M5** (D9, D16). |
| URL | `ws://{relay-host}:{port}/contract-b/v3` |
| Default port | `8795` (moved from M3's `8790` — see *The M4 port plan* below) |
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

### The M4 port plan

**The relay and the archive move out of the Contract A range, and the slots keep it.**
`contract-a.md` §10 gives the six-slot M4 rig the loopback range `8787`–`8792`, one Contract
A port per slot. M3's relay default `8790` is **slot 4's** port and M3's archive status-page
default `8791` is **slot 5's**, so on a six-slot rig both defaults collided with the range
they have to avoid — and `ringstat` inherited the second collision, because it defaults to
the archive's URL.

| Component | Port | Bind |
|---|---|---|
| Contract A, slots 1–6 | `8787`, `8788`, `8789`, `8790`, `8791`, `8792` | loopback only, by contract (`contract-a.md` §2) |
| Relay, Contract B | **`8795`** | `127.0.0.1` for a local rehearsal, `0.0.0.0` for the LAN |
| Archive status page and JSON | **`8796`** | loopback only. It is a **read** surface (§10.1) and M4 exposes nothing new |

These are the compiled defaults of `multiverse-relay`, `multiverse-sidecar`,
`multiverse-archive` and `ringstat` as of the M4 pre-flight, so a rig that passes no port
flags gets this layout. `--listen` / `MULTIVERSE_RELAY_LISTEN`, `--http` /
`MULTIVERSE_ARCHIVE_HTTP` and `MULTIVERSE_ARCHIVE_URL` still override each one.

**The alternative was five-digit slot ports**, which is what the M3 rig did for slot 3
(`18789`). Moving two components is a smaller change than moving six, and it keeps
`contract-a.md` §10's range as written.

**The far end dials the new port too.** A second computer set up against M3's `8790` will
not connect: the operator passes `-RelayPort 8795`, or takes a rebuilt bundle. The firewall
rule and the WSL portproxy on the relay's machine both name the port and both have to move
with it (`dev_environment.md`, *Owner steps*).

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
  "protocol": "contract-b/3.4",
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

**The species block is the one change since, and it is a minor** (added — §15, B10):
`MIGRATION_PAYLOAD.species` is an additive OPTIONAL field, so the identifier moves to
**`contract-b/3.1`** and every `contract-b/3.x` peer stays compatible with every other. A relay
or sidecar that does not know the field ignores it (§4, `contract-a.md` §3.1) and the
destination mod falls back to `contract-a.md` §16, A32 — quiet, defined degradation, never a
rejection.

**The species census is the second, and it is a minor for the same reason** (added — §16,
B12): the peer stats block gains `species` and `truncated` (§6.3.1), both additive and both
OPTIONAL, so the identifier moves to **`contract-b/3.2`**. A `contract-b/3.1` relay carries
them if it stores the block as it received it, and drops them if it re-encodes from a typed
model — §6.3.1 asks for the first (added — §16, B11) — and a peer that does not know them
simply omits them. Every one of those paths renders as **unknown** on the page (§10.1), and
none of them renders as a wrong census. No message type, enum, code, custody rule or routing
input changes.

**The reverse lanes are the third, and it is a minor for the third time** (added — §17, B15):
`SECTOR_GRANT.neighbours` gains the `"W"` and `"S"` keys (§6.4), which is **additive data in
an existing map keyed by an enum whose four values have been legal since M2** — no field is
removed, no type changes, no enum value is added or removed. The identifier moves to
**`contract-b/3.3`**. The degradation is the property that settles it: a `contract-b/3.2`
**relay** never computes the reverse walks, so a two-way sidecar receives no `W` or `S` key,
those two edges close with `no_peer` (§8), and the map runs exactly as it does today; a
`contract-b/3.2` **sidecar** ignores keys it did not declare (§6.4) and does the same. **Both
directions degrade to the one-way map, and neither loses an organism or misroutes one.** That
`MIGRATION_PAYLOAD.exitEdge` may now hold `"W"` or `"S"` is the same "existing enum value"
line the table above already ruled on for `"N"`.

**The pacing settings are the fourth, and it is a minor for the fourth time** (added — §18,
B17): the peer stats block gains `timeScale`, `inboundRatePerSimMinute` and
`inboundRateBurst` (§6.3.1), all three additive and all three OPTIONAL, so the identifier
moves to **`contract-b/3.4`**. A `contract-b/3.3` **relay** carries them for the reason §16's
B11 wrote down in advance — it stores the block as it received it rather than re-encoding it
from a typed model — and a peer that does not know them omits them. **Both paths render as
unknown on the page** (§10.1), and this is the one field group where "unknown" has to beat a
plausible substitute out loud: the shipped `inboundRatePerSimMinute` has changed three times,
so a reader that fills in the default is not degrading, it is reporting a different rig.

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

The relay **MUST NOT** decode `data.body.bb8`, **MUST NOT** decode `data.lineage`,
**MUST NOT** read `data.species` (added — §15, B9), and **MUST NOT** validate a payload
(D4 — that is `bb8-schema`'s job, sidecar-side, at both ends). It forwards the original frame
bytes unchanged. A species name is **never** a routing input, a filter, or an admission-control
term: the relay routes on `destSlot` and `destPeer`, and on nothing else.

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
| Fan-out covers the relay's own non-delivery answers | **The set is a superset of "routed"** (amended — §14, B7). Every `MIGRATION_NACK` the relay **generates in answer to a `MIGRATION_PAYLOAD` it declined to forward** — `SLOT_VACANT`, `PEER_OFFLINE`, `NOT_FORWARDED` (§6.8) — **MUST** also be fanned out. Those three are exactly the frames that carry `neverForwarded` and `relaySessionId`, so they are the only record a subscriber can ever have of a hop that reached no sidecar. The relay's two **connection-level** refusals are **not** fanned out, because no migration was in question: `NOT_A_MEMBER` (a payload from a subscriber, refused as a role error) and `PEER_UNKNOWN` (a routed `ACK`/`NACK` whose `destPeer` has gone). |
| Best effort | The fan-out **MUST NOT** delay, block or fail a migration. A subscriber that is absent, slow or dead changes nothing on the migration path. |
| Bounded | Each subscriber has a queue of `archiveQueueMax` (1024) frames. On overflow the relay **MUST** drop the **oldest** copy, increment a dropped-copies counter, and log at most one line per minute. It **MUST NOT** disconnect the migration it was copying. |
| No answers | A subscriber **MUST NOT** answer a copied frame. The relay ignores an `ACK`/`NACK` from a subscriber with one warning. |
| No sending | A `MIGRATION_PAYLOAD` from a subscriber is answered `MIGRATION_NACK` / `NOT_A_MEMBER` and is **not** forwarded. |
| What a subscriber may send | `HANDSHAKE`, `PING`, `PONG`, `GENOME_REQUEST`. Nothing else. |
| No claim | A `SECTOR_CLAIM` from a subscriber is refused with `granted: false, reason: "role_has_no_slot"`. |
| Duplicates | A re-forwarded or re-routed migration produces a second copy. The archive deduplicates a `MIGRATION_PAYLOAD` and a `MIGRATION_ACK` on `migrationId`, exactly as a sidecar does — and a **re-routed** copy carries a different `destSlot` with the same `migrationId`, which is not a duplicate organism but the same organism on a new lane (§6.6, §9). **A `MIGRATION_NACK` deduplicates on the pair `migrationId` + `code`** (amended — §14, B7), because one migration legitimately produces several different refusals on its way to a lane — a `PEER_OFFLINE` on the first attempt and a `NOT_FORWARDED` on the retry are two facts, not one fact twice, and collapsing them would erase the sequence a re-route has to be read against. |
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
| `protocolVersion` | string | yes | `"contract-b/3.4"` (amended — §18, B17; `"contract-b/3.3"` before it, §17 B15; `"contract-b/3.2"` before that, §16 B12; `"contract-b/3.1"` before that, §15 B10). A different **major** closes with `4000`. |
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
  "protocol": "contract-b/3.4",
  "type": "HANDSHAKE",
  "messageId": "9d1a4b77-2c60-4c1e-9f03-77a1c8e4b510",
  "sentAt": 1785693597011,
  "data": {
    "peerId": "peer-lan-slot5",
    "role": "peer",
    "protocolVersion": "contract-b/3.4",
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
| `protocolVersion` | string | yes | `"contract-b/3.4"` (amended — §18, B17; `"contract-b/3.3"` before it, §17 B15; `"contract-b/3.2"` before that, §16 B12; `"contract-b/3.1"` before that, §15 B10). |
| `relaySessionId` | `uuid` | yes | **New in M4.** Minted once at relay start, constant for the life of the relay process. It is the scope of the forwarding record (§5.2), and a sidecar **MUST** persist it against every journal entry it hands over while this connection is live (§9.2). |
| `assignedSlot` | number (int) | no | The slot this `peerId` already holds, when the relay remembers one. Absent for a first-time peer and always absent for an archive. |
| `assignedPosition` | object `{col,row}` | no | Its position. Present exactly when `assignedSlot` is. |
| `map` | object `{width,height}` | yes | The rectangle right now. `{"width":0,"height":0}` before the first placement. |
| `slotCount` | number (int) | yes | How many slots are reserved right now. `0` before the first placement. Renamed from M3's `ringSize`. |
| `receivedAt` | `timestampMs` | yes | The relay's own clock. Informational, and the anchor the archive uses to order records written by six machines' clocks. |

```json
{
  "protocol": "contract-b/3.4",
  "type": "HANDSHAKE_ACK",
  "messageId": "0b4e2a13-5d77-4b90-8a21-6f0c19d4e772",
  "sentAt": 1785693597019,
  "data": {
    "relayVersion": "0.4.0",
    "protocolVersion": "contract-b/3.4",
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
  "protocol": "contract-b/3.4",
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
      },
      "species": [
        { "genericName": "Izus ",      "specificName": "copedylanus", "bibites": 96, "eggs": 14 },
        { "genericName": "Cyanea",     "specificName": "velox",       "bibites": 61, "eggs":  9 },
        { "genericName": "Alvaradus",  "specificName": "powerus",     "bibites": 38, "eggs": 11 },
        { "genericName": "Banagellus", "specificName": "polatus ",    "bibites": 17, "eggs":  3 }
      ]
    }
  }
}
```

That census is the one `contract-a.md` §5.2's `HEARTBEAT` example carries, copied byte for
byte (added — §16, B11). **`"Izus "` and `"polatus "` keep their trailing spaces here**: that
world's registry holds `Izus  copedylanus` with a doubled space and its player sees it that
way, and a sidecar that tidied the copy would be reporting a world that does not exist. The
same species travelling on a `MIGRATION_PAYLOAD` would carry `"Izus"`, normalized at the
source — two lanes, two rules, one `Species` record (§6.6, `contract-a.md` §17, A36).

A brand-new instance asking to extend a full 3×2 map into a fourth column — the exit test's
Part 4, in one frame:

```json
{
  "protocol": "contract-b/3.4",
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
    "stats": { "population": 0, "custodyDepth": 0, "pacedDepth": 0, "heldDepth": 0,
               "species": [] }
  }
}
```

**`"species": []` there is a statement, and it is not the same as omitting the field**
(added — §16, B11). That instance has a mod, the mod speaks `contract-a/2.2`, and it is
reporting that nothing is alive in its world yet. Omitting the field would have said *unknown*
— no mod, an older mod, or no heartbeat yet — and the status page renders the two
differently (§10.1).

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
| `timeScale` | float | no | **How fast the world is running** (added — §18, B16), copied from the last `HEARTBEAT.timeScale` (`contract-a.md` §5.2) and **never computed here**. `5` is five simulated seconds per real second; `0` is a world standing still, which is a **reading and not a gap** — a reader renders it as such and does not confuse it with absence. Absent means **unknown**: no mod is connected, or no heartbeat has arrived on this session yet. It is the interpretive key to `simulatedTime` and to the two fields below: both advance and are spent in the world's own time, not the wall's. |
| `inboundRatePerSimMinute` | float | no | **The delivery rate limit this sidecar is configured with** (added — §18, B16), from `contract-a.md` §7.5 and its `--inbound-rate` knob (§18, A40). A **setting**, not a measurement, and the sidecar always knows its own — so absence here means the peer's build predates this field, never that it has no limit. **A reader MUST render an absent value as unknown and MUST NOT substitute the shipped default**, which has been changed three times (2.0 → 12.0 → 100.0 on 2026-08-07 alone). It is what makes `pacedDepth` readable: twelve entries queued behind 120 a simulated minute is a blink, and twelve behind 2.0 is six minutes. |
| `inboundRateBurst` | float | no | The token-bucket capacity behind that rate (added — §18, B16), same source and same rules. It bounds the largest clump the pacer can release at once, so a `pacedDepth` that sits below it is a queue that has never actually been paced. |
| `lastSave` | object | no | The mod's save receipt, copied verbatim from `HEARTBEAT.lastSave` (`contract-a.md` §5.2, §15 A21): `atMs`, `simulatedTime`, `population`, and the optional `name`, `bytes`, `durationMs`. |
| `species` | array of object | no | **The world's active species census** (added — §16, B11), copied **verbatim** from the last `HEARTBEAT.species` the sidecar received (`contract-a.md` §5.2, §17 A35). One entry per species with at least one living member or egg, sorted by `bibites + eggs` descending, at most `speciesCensusMax` (32) entries. Absent means **unknown** — no mod is connected, the mod predates `contract-a/2.2`, or no heartbeat has carried one. A present `[]` means a reporting mod with nothing alive in its world, which is a different fact (§10.1). |
| `species[].genericName` | string | yes | The genus half, **raw**: the bytes the origin world's `Species` record holds. Valid UTF-8, 1 to 64 UTF-8 bytes. Leading, trailing and doubled internal whitespace are **legal here**, and no party on this wire may trim, collapse, case-fold, normalize or re-case one. **This is deliberately not the rule `MIGRATION_PAYLOAD.species` carries** (§6.6, §15 B9): that name is a matching key the exporting mod normalizes at the source (`contract-a.md` §16, A34); this one is a display label that must read as the owning world's player sees it (`contract-a.md` §17, A36). |
| `species[].specificName` | string | yes | The specific half. Same rules. The world's display name is `genericName + " " + specificName`. |
| `species[].bibites` | number (int) | yes | Living members of that species in that world, `≥ 0`. Excludes eggs, so it is on the same footing as `population`. |
| `species[].eggs` | number (int) | yes | Unhatched eggs of that species, `≥ 0`. `bibites + eggs` is the game's own `Species.count`. |
| `truncated` | bool | no | **Qualifies `species` and nothing else** (added — §16, B11). `true` means the array is **not the whole census** — the mod hit the cap, or a sidecar stripped an entry or trimmed an over-long array (`contract-a.md` §5.2). Monotonic: set on the way, never cleared. Ignored when `species` is absent. |

**Every field is optional, and absence is a value.** A stat the sidecar does not know is
omitted, never defaulted. The status page renders an omitted field as **unknown**, which is
Risk 4's rule: a slot that reports nothing is unknown, not empty, and an honest gap beats a
confident zero.

**The relay does not interpret any of it.** It stores the last block per peer with the time
it arrived, republishes it, and never routes, schedules, refuses or filters on a stat. D1's
dumb relay survives: this is one more field copied into a broadcast it was already sending.
**That sentence already covers the census in full** (added — §16, B11) — a species name is
never a routing input, a filter, an admission-control term or a scheduling term, exactly as a
population is not — and B11 adds no relay rule to it. What it adds is one **SHOULD**: a relay
**SHOULD** store the stats block as the bytes it arrived as, rather than re-encoding it from a
typed model, so a field a newer sidecar sends survives an older relay. It is a forward-
compatibility habit, not a new behaviour.

**The census is what bounds this block's size, and the cap is why** (added — §16, B11). A full
32-entry census is about 3 KB; a typical rig world reports four to a dozen species and under
1 KB. A stats-bearing `PING` carries one, at `statsIntervalMs` (5 s), and a `PEER_STATUS`
carries one **per slot** — so a six-slot map broadcasts at most ~20 KB every
`statsBroadcastIntervalMs`, and even a 32-slot map broadcasting full censuses stays two orders
of magnitude inside `maxFrameBytes`. `speciesCensusMax` (`contract-a.md` §10) is the constant that
makes that arithmetic hold, and no party on this wire may raise it unilaterally.

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
| `neighbours` | object | no | Present when `granted` is `true`. Keyed by **export edge**: `"E"`, `"N"` — and, under two-way lanes, also `"W"` and `"S"` (amended — §17, B13). **A key is absent when that edge has no deliverable target**, and its absence is what closes that export edge with `no_peer` (§8). The relay emits a key for **every edge the sidecar declared** in `SECTOR_CLAIM.exportEdges` and finds a target for, and for no other; a sidecar **MUST** ignore a key for an edge it did not declare, and **MUST NOT** treat an absent key as an error. |
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
  "protocol": "contract-b/3.4",
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

The same grant to the same peer once it declares all four edges (added — §17, B13). Its `W`
walk goes the other way along row 1 — `(2,1)` first, then `(1,1)` — so it reaches slot 6
**without skipping anything**, while the `E` walk had to step over dead slot 5. Its `S` key is
slot 1, the same peer its `N` key names, because the column has height 2:

```json
    "neighbours": {
      "E": { "slot": 6, "peerId": "peer-main-slot6", "position": { "col": 2, "row": 1 },
             "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
             "simulationSize": 2000.0,
             "skipped": [ { "slot": 5, "position": { "col": 1, "row": 1 },
                            "reason": "peer_offline" } ] },
      "N": { "slot": 1, "peerId": "peer-main-slot1", "position": { "col": 0, "row": 0 },
             "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
             "simulationSize": 2000.0, "skipped": [] },
      "W": { "slot": 6, "peerId": "peer-main-slot6", "position": { "col": 2, "row": 1 },
             "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
             "simulationSize": 2000.0, "skipped": [] },
      "S": { "slot": 1, "peerId": "peer-main-slot1", "position": { "col": 0, "row": 0 },
             "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
             "simulationSize": 2000.0, "skipped": [] }
    }
```

Two properties of that frame are worth naming because they look like mistakes and are not.
**`E` and `W` can name the same slot with different skip lists** — the two walks meet the same
peer from opposite directions, and the skip list describes the walk, not the target. **`N` and
`S` name the same slot with identical skip lists** — that is §2.1's degenerate axis, and on
this rig every column has it.

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
  "protocol": "contract-b/3.4",
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
        "stats": { "population": 231, "custodyDepth": 1, "pacedDepth": 0, "heldDepth": 0,
                   "species": [
                     { "genericName": "Izus ",      "specificName": "copedylanus", "bibites": 104, "eggs": 19 },
                     { "genericName": "Cyanea",     "specificName": "velox",       "bibites":  72, "eggs": 11 },
                     { "genericName": "Alvaradus",  "specificName": "powerus",     "bibites":  41, "eggs":  6 },
                     { "genericName": "Banagellus", "specificName": "polatus ",    "bibites":  14, "eggs":  2 }
                   ] },
        "statsAsOfMs": 1785693731644 },
      { "slot": 2, "position": { "col": 1, "row": 0 }, "peerId": "peer-main-slot2",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731641,
        "stats": { "population": 208, "custodyDepth": 0, "pacedDepth": 0, "heldDepth": 0,
                   "species": [ ... 7 entries, elided ... ] },
        "statsAsOfMs": 1785693731641 },
      { "slot": 3, "position": { "col": 2, "row": 0 }, "peerId": "peer-main-slot3",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731639,
        "stats": { "population": 197, "custodyDepth": 0, "pacedDepth": 0, "heldDepth": 0,
                   "species": [ ... 5 entries, elided ... ] },
        "statsAsOfMs": 1785693731639 },
      { "slot": 4, "position": { "col": 0, "row": 1 }, "peerId": "peer-lan-slot4",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731630,
        "stats": { "population": 244, "custodyDepth": 3, "pacedDepth": 11, "heldDepth": 2,
                   "species": [ ... 32 entries, elided ... ], "truncated": true },
        "statsAsOfMs": 1785693731630 },
      { "slot": 5, "position": { "col": 1, "row": 1 }, "peerId": "peer-lan-slot5",
        "live": false, "modConnected": false, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693719004,
        "darkSinceMs": 1785693719004,
        "stats": { "population": 226, "custodyDepth": 0, "pacedDepth": 0, "heldDepth": 0,
                   "species": [ ... 9 entries, elided ... ] },
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

**The censuses in that frame carry four different statements, and a page has to tell them
apart** (added — §16, B11). `[ ... n entries, elided ... ]` is this document's elision, not a
wire form — every one of those is an ordinary array:

| Slot | What its census says |
|---|---|
| 1 | A complete census. Its four `bibites` counts sum to `231`, which is exactly its `population`: every organism in that world has a `Species` record. `"Izus "` keeps its trailing space, because that world's registry does. |
| 2, 3, 5 | Complete censuses of 7, 5 and 9 species. Slot 5's is **as stale as its population** — `statsAsOfMs` ages the whole block, and a page that greys out one number and not the other is lying about the same instant twice. |
| 4 | `truncated: true`: that world holds more than `speciesCensusMax` species and the block names the **32 most abundant**. A page **MUST NOT** present it as the world's whole species list, and its `bibites` sum is below `population` by construction. |
| 6 | **No `species` key at all** — that peer's mod predates `contract-a/2.2`. It renders as **unknown**, never as "no species" and never as zero, and every other stat that slot reports stays exact (§10.1). |

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
| `species` | object | no | **The migrant's species identity, opaque to this wire** (added — §15, B9). Copied verbatim out of the origin mod's `MIGRATE_OUT.species` (`contract-a.md` §5.3, §16 A30) and handed verbatim to the destination mod on `MIGRATE_IN`. Absent is valid and ordinary: an organism with no species record, a mod that does not implement `contract-a/2.1`, or a block a schema check stripped. |
| `species.genericName` | string | yes | The genus half of the species name. REQUIRED when `species` is present. Non-empty, at most **64 UTF-8 bytes**, carried byte for byte — no trimming, no case folding, no normalization. **This wire's rule is unchanged by `contract-a.md` §16 A34**: the *exporting mod* normalizes a name's whitespace at the source, before it ever reaches a sidecar, and nothing on this wire may repair one. |
| `species.specificName` | string | yes | The specific half. Same rules. The destination mod matches on `genericName + " " + specificName` with exactly one U+0020 between them (`contract-a.md` §5.7 step 3). |
| `species.parentGenericName` | string | no | The genus half of the **immediate parent species'** name, when the origin's species had a parent. |
| `species.parentSpecificName` | string | no | The specific half of it. **All-or-nothing with the field above**: both present or both absent. Only one generation travels. |
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

**The `species` block is carried, validated for shape, and never interpreted** (added — §15,
B9). Six rules, and they are the whole of this wire's involvement:

| Rule | Statement |
|---|---|
| Copy, never author | The source sidecar copies the block out of `MIGRATE_OUT` unchanged. It **MUST NOT** synthesize one, fill a missing half, infer a parent, or derive anything from `body.bb8` — the payload holds a world-local integer and no name at all (`contract-a.md` §16). |
| Schema validation only | Both sidecars check the shape stated above and nothing more. A **malformed** block — a missing half, a lone parent field, a non-string, an over-long name — is **stripped, logged once, and the migration proceeds without it**. It is never a `MIGRATION_NACK` reason, never a validation failure, and never a reason to hold an organism (`contract-a.md` §9.1). |
| Opaque to routing | It takes no part in dedup, custody, admission control, the `S` check, the genome hash, the forwarding record, the hold clock, or a re-route. A **re-routed** frame carries the same block byte for byte, exactly as it carries the same `migrationId` and the same body. |
| The relay reads nothing | §5's list now names `data.species`. A relay that filtered or routed on a species name would be a relay with an opinion about biology. |
| Delivered verbatim | The receiving sidecar hands the block through to its mod on Contract A `MIGRATE_IN` (step 7 below) with every byte intact. It **MUST NOT** resolve, translate, or annotate it: the registry the name resolves against lives inside the game process, and only the mod can see it. |
| Recorded | The archive records it against the migration (§10, added — §15, B10). That is the only place on this wire a species name is *used* for anything, and it is off the migration path by construction. |

**Why a top-level field and not part of `lineage`.** The annex is the genome graph: hashes,
parents, gaps, all of it computed by the sidecar under `genome-hash.md`. A species name is
neither computed here nor hashed here — it is metadata the origin *world* asserts, the way
`exitEdge` is geometry the origin world asserts. Putting an uncomputed, unverifiable string
inside the annex would invite an implementer to treat it as a derived value and to
reconcile it against a hash that deliberately excludes it (`genome-hash.md` §4.3).

**Contract debt — the sidecar can only emit two of the three `gapReason` values.** A parent
the mod dropped for frame size arrives on Contract A as an `entityId` with no `payload`,
which is byte for byte what a dead parent looks like, so the sidecar records `"parent_gone"`
and `"blob_dropped_for_size"` never appears on this wire. The value stays in the enum: the
mod logs each drop on its own side, a receiver must already tolerate every value here, and
removing it would be a wire change for a field a reader must handle defensively anyway.
Closing the debt needs one additive optional flag on Contract A's `parents[]`, and M4 does
not add it either (`contract-a.md` §14 A12, §12 item 8).

The receiving sidecar **MUST**, in this order:

1. Deduplicate on `migrationId` against its journal **and its tombstones**. **A hit delivers
   nothing** (`contract-a.md` §7.2). "In this order" includes the decode: the receiver
   extracts `migrationId` with a minimal parse and answers a hit **before** it decodes
   `body.bb8`, so a duplicate costs an O(1) lookup, never a multi-MiB unmarshal — and a
   duplicate whose body would not even parse is still answered by this table, because the
   question is the organism's custody, not the frame's validity (amended — §14, B8).
   Whether a hit is also *answered* depends on which kind of
   hit it is, and the two cases are not the same (amended — §14, B6):

   | The hit | Answer |
   |---|---|
   | A **tombstone** — this `migrationId` was delivered to the mod and ACKed once already | `MIGRATION_ACK` immediately, with `duplicate: true`. Delivery is proven, so re-stating it says nothing new. |
   | A **journal entry that is not yet a tombstone** — journaled, and still waiting for its mod, whether behind the delivery rate limit or awaiting `MIGRATE_IN_ACK` | **Nothing.** Silence, and one log line. §6.7 forbids an ACK before the receiving mod's `MIGRATE_IN_ACK`, and this is that rule, not an exception to it. |

   **The second row is the one that matters, and pacing is why** (Risk 9). A paced delivery
   can sit in this journal for minutes of simulated time. ACKing it here would release the
   sender's custody at the moment the organism is *least* delivered — journaled at the
   receiver, not yet spawned — and if that receiver then dies, both sides have let go. The
   sender's retry costs nothing: it lands right back in this branch, and §9.3's hold clock
   does not run while the destination is live.

   **A re-routed frame deduplicates on exactly the same key**, which is what makes a re-route
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
   **Copy `species` through unchanged when the envelope carried one** (added — §15, B9); when
   it did not, send none, and the mod applies its absent-block rule (`contract-a.md` §16, A32).
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
  "protocol": "contract-b/3.4",
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
    "species": {
      "genericName": "Cyanea",
      "specificName": "velox",
      "parentGenericName": "Cyanea",
      "parentSpecificName": "prima"
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
geometry and the species block are untouched, and **only `destSlot` changed** — plus the
`reroute` block that says so:

```json
{
  "protocol": "contract-b/3.4",
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
    "species": {
      "genericName": "Cyanea",
      "specificName": "velox",
      "parentGenericName": "Cyanea",
      "parentSpecificName": "prima"
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
tombstoned was ACKed once before, so re-ACKing it carries the same meaning. **A duplicate
that is merely journaled is not that case and is answered with nothing** — §6.6 step 1's
table is aligned to this section, and this section is the authority (§14, B6).

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
  "protocol": "contract-b/3.4",
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
  "protocol": "contract-b/3.4",
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
  "protocol": "contract-b/3.4",
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
  "protocol": "contract-b/3.4",
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
  "protocol": "contract-b/3.4",
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
  "protocol": "contract-b/3.4",
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
      },
      "species": [
        { "genericName": "Izus ",      "specificName": "copedylanus", "bibites": 96, "eggs": 14 },
        { "genericName": "Cyanea",     "specificName": "velox",       "bibites": 61, "eggs":  9 },
        { "genericName": "Alvaradus",  "specificName": "powerus",     "bibites": 38, "eggs": 11 },
        { "genericName": "Banagellus", "specificName": "polatus ",    "bibites": 17, "eggs":  3 }
      ]
    }
  }
}
```

**This is the frame the census actually rides on** (added — §16, B11), for the same reason
`population` does: a `SECTOR_CLAIM` is sent on change, so a map view fed only by claims would
name the species a world held when it last resized. The block is the one the sidecar last
received on `HEARTBEAT`, copied verbatim — the sidecar re-sorts nothing, merges nothing,
renames nothing and drops nothing that passed its Contract A shape check
(`contract-a.md` §5.2).

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

                                                     added — §17, B13 (D17):
effective(W) = first deliverable slot at (col−1, row), (col−2, row), … mod width
effective(S) = first deliverable slot at (col, row−1), (col, row−2), … mod height
```

Each walk visits every other position on its axis exactly once and then stops. The relay
performs it and publishes the result in `SECTOR_GRANT.neighbours` (§6.4); a sidecar uses the
grant, and any client may reproduce the walk from `PEER_STATUS` for display (§6.5).

**The two new walks are the two old walks with the step negated, and that is the whole of
D17 on this wire** (added — §17, B13). Everything else composes for free and is stated so no
implementer re-derives it:

| Property | Under two-way lanes |
|---|---|
| `deliverable()` | **Identical.** The same six conditions, including `slot ≠ me`, filter all four walks. |
| Skip lists | **Per walk, not per axis.** `E` and `W` traverse the same row and can produce different `skipped` lists, because they meet the dark slots in a different order and stop at the first live one. |
| Termination | Every walk still visits `length − 1` other positions and stops, so an axis of length 1 has no candidates in either direction and both edges close with `no_peer`. |
| Axis of length 2 | `effective(E)` and `effective(W)` name the **same** slot, because ±1 mod 2 are the same position (§2.1). Both close together. |
| The close mapping | The table below is **unchanged and applies per edge**, not per axis. A closed `W` reports the aggregate of the `W` walk's own skip list. |
| The ripple | **Unchanged in mechanism, doubled in reach.** A slot going dark is still announced backwards along every lane that pointed at it — there are now up to four such lanes per neighbour instead of two. |
| `PEER_STATUS` | **Unchanged.** It publishes the structural row-major order and the relay's registry facts; the lanes have always been derived from it, and two more derivations need no new field. |

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
- ~~the dark slot's own **east and north** neighbours are unaffected and are told nothing; they
  simply receive nothing from it;~~ **Superseded — §17, B13.** Under two-way lanes every
  neighbour of a dark slot both pointed at it and was pointed at by it, so **the ripple is
  symmetric**: the peers that used to be told nothing are now in the first bullet like
  everyone else. The rule is unchanged in mechanism — *announce backwards along each lane
  that pointed at it* — and the set it applies to has grown to every lane;
- the dark slot's reservation stays in the map, `live: false`, with `darkSinceMs` set.

**What that costs, and why it needs no new coalescing** (added — §17, B13). One liveness flip
now re-grants up to twice as many peers. `statusCoalesceMs` (250) already bounds the burst —
it spaces both `PEER_STATUS` broadcasts and per-peer grants, and always sends the last frame
of a burst — and a grant is re-sent only when its **content** changes (§6.4), which a lane
that did not re-target does not. The existing mechanism absorbs this; it is named here so
nobody adds a second one.

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
| Forwarded, then silence | none | **Hold, then bounce** (§9.3). Retry the recorded `destSlot` on the retry cadence — flat toward a dark destination, backing off exponentially toward a live one (amended — §14, B8); the retry is idempotent because the destination deduplicates. |

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
| **The retry keeps running** | A `held` entry **MUST** keep re-forwarding its **recorded** `destSlot` on the ordinary `forwardRetryMs` cadence, dark destination and all (added — §14, B5). The retry does **not** reset the handoff state, and it does **not** reset or pause the accrual. §9.2 row 4 states the same rule from the other side; it is repeated here because this is the section an implementer reads while building the hold, and a hold that stops retrying is the natural thing to build. The **dark** cadence stays flat because this retry is the proof's only conduit; it is the retry toward a **live** destination that backs off (§14, B8). |
| **When it runs** | Only while **all three** hold: the entry is in `held`; the sender has a **live relay connection**; and the destination slot is, in the sender's latest `PEER_STATUS`, **not live** (`live: false`) or **absent from the map** entirely. |
| **When it stops** | The moment any of those stops being true. The destination coming back stops it. The **sender** losing its own relay connection stops it. A relay restart stops it until the sender reconnects and sees the map again. |
| What it never counts | Time while the destination is **live**. A live peer with a deep paced backlog is **slow, not orphaned** (Risk 9), and pacing can make a live peer silent for a long simulated while. Counting that time would bounce an organism that was about to be spawned — and that is a duplication, not a delay. |
| What it never counts, either | Time while the **sender** is blind. A sender whose own machine slept for a night must not wake with an expired clock: it never observed the destination dark, it only failed to observe anything. T1 was exactly that machine. |
| Where it lives | In the journal entry, as **accrued milliseconds**, flushed at least every `holdAccrualFlushMs` (60 000) and on clean shutdown. On restart the sidecar **resumes the accrual** and **MUST NOT** start a fresh timer. |
| Why accrual and not a deadline | `m4_considerations.md` says "the deadline lives in the journal entry" and means "the timer survives a restart and never resets". A wall-clock deadline cannot express a clock that stops, so the entry carries the accrual instead. It satisfies the requirement more strictly: a restart cannot lose time already served, and cannot invent time that was never served. |
| A crash | Loses at most one flush interval of accrual, in the safe direction: the entry waits slightly longer. |
| At the timeout | The sidecar **bounces the organism home by itself** — an ordinary Contract A `MIGRATE_IN` with `bounceBack: true` into its own mod, on the edge it left from (§9.4) — records the reason on the entry, increments `stats.bouncedTimeoutTotal` (§6.3.1), and logs one loud line. |
| Reporting | **An automatic bounce is a fact the operator reads, not a silent repair.** The status page and `ringstat` both name every bounce a timeout caused, with the migration, the entity, the destination slot and the accrued hold. |

**The retry is the only conduit the proof has, and without it `held` is a trap** (§14, B5).
The relay never volunteers a `neverForwarded` statement. It mints one **only** in answer to a
`MIGRATION_PAYLOAD` it declined to forward (§5.2, §6.8), so a sender that stops sending stops
being told anything. Follow the consequence through: a sender that hands the relay a frame
for a slot that is *already* dark has an entry in `sent`, then `held`, while the relay's
forwarding record holds nothing for that `migrationId` — the proof of non-delivery exists and
is waiting, and the only way to collect it is to ask again. Stop retrying and
`held → refused → re-route` becomes unreachable: **every** held entry then runs the full
24-hour clock and bounces, including the large majority that could have re-routed in seconds.
That is not a degraded version of route-around. It is route-around switched off for exactly
the case it was built for.

The retry is safe to run into the dark for the same reason it is safe anywhere: the
destination deduplicates on `migrationId` (§6.6 step 1), so a delivery that did happen is
answered as a duplicate and a delivery that did not is answered with the proof.

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
| The forward reached a live peer and no answer came back, and the destination is **live** | **Hold and re-forward.** Never bounce. The destination deduplicates, so a re-forward is free, and holding converts a possible duplication into a bounded delay. The re-forward cadence backs off exponentially to `forwardRetryMaxMs` — free is not the same as costless at scale (amended — §14, B8). |
| The forward reached a live peer and no answer came back, and the destination is **dark** | **Hold with the clock of §9.3**, then bounce at the timeout. |

A bounce re-delivers the organism to the origin's own mod as a Contract A `MIGRATE_IN` with
`bounceBack: true`, **on the edge it left from** — the origin's own `exitEdge`, `"E"` or
`"N"`, not a passive entry edge. It lands `entryMargin` **inside** that edge's capture band
rather than in it, because `contract-a.md` §4.3 insets and clamps every spawn coordinate
(`contract-a.md` §15 A28) — but it lands there still **moving outward**, because velocity is
copied and never mirrored, so it re-enters the band under its own power within a few ticks. The
entry-immunity window is what stops the round trip from resolving as a second export, and
`contract-a.md` §14 A11, §15 A19 and §15 A28 make that window REQUIRED for exactly this
reason.

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

**Species names on the record** (added — §15, B10). The archive records the `species` block of
§6.6 against the migration it arrived on, and that is the only place on this wire a species
name is used for anything.

| Rule | Statement |
|---|---|
| Recorded verbatim | Both names, and the parent pair when present, stored byte for byte as they arrived. The archive **MUST NOT** trim, case-fold, normalize or re-case a name; the destination mod's match is a byte comparison (`contract-a.md` §5.7), and a ledger that tidied its copy would stop describing what actually happened. |
| A ledger fact, not a resolution | The block says what the **origin world** called this migrant at the moment it left. It is not a claim about the destination, and the archive **MUST NOT** resolve a name against any world's registry, merge two records because their names match, or rewrite an earlier record when a later envelope disagrees. Species resolution happens in exactly one place in this system, and it is the importing mod. |
| Absent is absent | A migration with no block records **no species** — never `"unknown"` as a value, never an empty string standing in for one. That is §10.1's unknown rule applied to a new field. |
| Dedup unchanged | A `MIGRATION_PAYLOAD` still deduplicates on `migrationId` alone (§5.1). The block is part of the record that key names, never part of the key. |
| The win | Before this the ledger could name only hashes, so "which species crossed, and when" was a question the archive held the data to answer and not the labels. It now travels on every hop that carries a block. |
| Not the page's species view | **Amended — §16, B12.** The page *does* render species now, and **not from here**: its species view is the live census in `stats.species` (§6.3.1), which describes who lives in a world. This ledger describes who **crossed**, and it remains **not an input to any abundance claim** — a database built from migrations holds migrants and their ancestors, never a resident population (D11, below). The two also spell a name differently on purpose: the ledger's copy is normalized at the source (`contract-a.md` §16, A34) and the census's is raw (`contract-a.md` §17, A36), so a consumer that joins them normalizes **for the comparison only** and rewrites neither. |

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
| The census is a stat, and every rule above applies to it (added — §16, B12) | `stats.species` (§6.3.1) is what the page's species view is built from. Absent renders as **unknown species** — never as "no species", never as an empty list, never as zero. A present `[]` is the different, stronger fact and the page may say so: a reporting world with nothing alive in it. A `truncated: true` census names the 32 most abundant species and the page **MUST** say the rest is unreported rather than presenting it as the whole list. And it ages like everything else in the block: past `statsStaleMs` it is history, not state. |
| Two species facts, two sources, and only one of them is abundance (added — §16, B12) | The **census** says what lives in a world **now** and arrives on `PEER_STATUS`. The archive's **ledger** of `MIGRATION_PAYLOAD.species` (§10) says what **crossed**, and when. The page's species view comes from the census alone: a migration ledger holds migrants and their ancestors, never a resident population (D11), so answering "which species live in slot 4" from it produces a plausible-looking wrong number. A page that shows both **MUST** label which question each answers, and **MUST NOT** join them on a name without normalizing the census copy for the comparison only (`contract-a.md` §17, A36). |
| A world's speed and its pacing are settings, and unknown beats the default (added — §18, B17) | The page may show each world's `timeScale` and the `inboundRatePerSimMinute` / `pacedDepth` pair (§6.3.1), because a depth is only readable against the cap it is queued behind and a simulated-minute cap is only readable against the speed that spends it. Every rule above binds them, and **the unknown rule binds them hardest**: a peer that publishes no cap renders as **unknown**, never as the shipped default. `timeScale: 0` is the opposite case and the page **MUST** keep the two apart — a world standing still is a reading, and a world that has not said is a gap. |
| The recent-hops feed is ledger, and it animates rather than counts (added — §17, B14) | The page may show **which species crossed which lane, just now**, as a bounded feed of the last ~60 seconds drawn from the `MIGRATION_PAYLOAD` copies the archive already records. It is B12's third row exercised — *history, labelled as history* — and every rule above binds it: a hop whose envelope carried **no** species block renders as the **neutral glyph**, never a guessed name and never omitted; the feed is **never summed**, into a census or into anything else; and it must be bounded in **both** time and count, because the status view is serialized verbatim into the durable metrics file once a minute. |

**What changed here, and what did not** (amended — §16, B12). §15's B10 stated that the
species names the archive records are **not an input to this page in M4**, and that the
rendering it anticipated was unspecified. **The rendering has arrived, and it does not come
from the ledger.** The page's species view is the live census in `stats.species`, which
travels the same path as `population` and obeys every rule in the table above. The ledger's
names stay exactly where B10 put them: a record of what crossed, never a source for what
lives. Nothing else in this section's list changes.

The exit test asks the page for the map and its holes, each slot's liveness and population,
each effective lane, each bypass with the time it went dark, each sidecar's custody depth,
each bounce a hold timeout caused, the last save of each world, and the paced journal depth —
**since §16, which species live in each world and in what numbers**, and **since §18, how
fast each world runs and the cap its arrivals are paced behind**. Every one of those
is a field of §6.5 or §6.3.1, or is derived from them by §8 — which is why those two sections
carry fields that no routing decision reads.

**Since §17 it also asks the page to show the lanes moving** (added — §17, B14). When a
migration lands, the map animates that species' glyph along the lane's arrow. The inputs are
not new — the archive has recorded a species block on every migration since §15's B10, and it
has counted per-lane hops since M4 — and no wire field is added. What is new is the join, and
it is the cheapest verification the operator surface has ever had: **two-way lanes look like
glyphs travelling both ways along one arrow, and an excluded species (`contract-a.md` §18,
A39) looks like a species that is everywhere in the census and never on a lane.**

**And since §18, everything that moves on that map is evidence** (added — §18, B17). The page
also drew an **ambient pulse** per lane — a dot walking the arrow at the lane's measured rate,
which named no organism and said nothing the lane's own `/min` label did not say in a number a
reader can compare. Once real hops travelled the same arrows it was worse than redundant: two
things moving along one lane, one of them a migration this archive was copied on and one of
them decoration, drawn at the same size on the same path. **It is removed.** The rate stays,
as the number it always was. This is a page decision and not a wire one — no field, no
message, no component learns of it — and it is recorded here for the same reason B14 was:
§10.1 is where what the page may claim is written down, and "a moving glyph is a real
crossing" is now one of those claims.

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
| `relayPort` | `8795` | both | The relay's listen port. Opened in the Windows Firewall on the relay's machine. **Moved from M3's `8790` in M4**, which is slot 4's Contract A port on a six-slot rig — see *The M4 port plan* (§3). |
| `archiveHTTPPort` | `8796` | archive | The status page and its JSON endpoint, loopback only (§10.1). **New in M4.** Moved off `8791`, which is slot 5's Contract A port. `ringstat` defaults to the same address. |
| `relayPingIntervalMs` | `5000` | relay | Application-level `PING` cadence. |
| `peerTimeoutMs` | `15000` | relay | Silence before a peer is dropped and its slot goes `live: false`. |
| `relayBackoffMinMs` | `1000` | client | Reconnect floor. |
| `relayBackoffMaxMs` | `30000` | client | Reconnect ceiling. |
| `stableSessionMs` | `5000` | client | How long a connection must live before the backoff ladder resets (`contract-a.md` §13, A8). |
| `authFailuresBeforeCeiling` | `5` | client | Consecutive HTTP 401s before the backoff is pinned at the ceiling (§3.1). |
| `statusCoalesceMs` | `250` | relay | Minimum spacing between `PEER_STATUS` broadcasts, and between grants to one peer. The last frame of a burst is always sent (§7.2). |
| `statsBroadcastIntervalMs` | `5000` | relay | **New in M4** (added — §14, B4). The §6.5 timer that republishes `PEER_STATUS`, and re-sends any changed `SECTOR_GRANT`, because **stats change without the registry changing**. §6.5 named it and this table did not define it. Set to `statsIntervalMs`, the cadence at which the stats it carries arrive: a faster timer would republish the same block, a slower one would age it. It is a compiled default with no flag and no environment variable. |
| `statsIntervalMs` | `5000` | sidecar | Minimum spacing between stats-bearing `PING`s (§6.11). |
| `statsStaleMs` | `30000` | archive | Age at which a `stats` block renders as unknown rather than as state (§10.1). |
| `forwardRetryMs` | `5000` | sidecar | Re-forward cadence for a journaled outbound entry with no answer — flat toward a dark destination, and the backoff floor toward a live one (§14, B8). |
| `forwardRetryMaxMs` | `60000` | sidecar | **New after M4** (added — §14, B8). Ceiling of the doubling re-forward backoff toward a **live** destination that has not answered (§9.2, §9.4). Resets on any state change or liveness flip. A dark destination keeps the flat `forwardRetryMs` (§9.3, B5). |
| `bounceTimeoutMs` | `20000` | sidecar | How long an outbound entry that never reached a live peer, **and has no lane to re-route to**, waits before it is bounced home (§9.2, §9.4). |
| `migrationAckTimeoutMs` | `30000` | sidecar | Informational deadline for `MIGRATION_ACK`; expiry re-forwards, it never bounces (§9.4). |
| `holdTimeoutMs` | `86400000` | sidecar | **New in M4.** Accrued dark time before a held entry bounces home by itself — 24 hours (D2, signed off 2026-08-05). The clock runs only while the destination is dark and the sender can see it (§9.3). |
| `holdAccrualFlushMs` | `60000` | sidecar | **New in M4.** How often the accrued hold time is flushed to the journal entry. A crash loses at most this much, in the safe direction (§9.3). |
| `maxReroutes` | `4` | sidecar | **New in M4.** Re-routes one entry may take before it bounces home instead (§9.2). |
| `forwardRecordRetentionSeconds` | `172800` | relay | **New in M4.** How long the relay remembers a forwarded `migrationId`, in memory, for the `neverForwarded` proof — 48 hours, twice the default hold (§5.2). |
| `inboundQueueMax` | `64` | sidecar | Shared with `contract-a.md` §10. Also the ceiling a paced backlog hits, which is what turns pacing into upstream backpressure (`contract-a.md` §7.5). |
| `exportRetentionSeconds` | `3600` | sidecar | Tombstone lifetime. Shared with `contract-a.md` §10. |
| `speciesCensusMax` | `32` | both | **New after M4** (added — §16, B11). Shared with `contract-a.md` §10. Upper bound on `stats.species` entries. A block that arrives with more is trimmed to the first 32 with one warning and `truncated: true`, never a refusal and never a close — the same answer `contract-a.md` §5.2 gives on the other wire. It is what bounds a `PEER_STATUS` broadcast carrying one census per slot (§6.3.1). |
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

**It was raised twice on 2026-08-07, and this wire is why it had to be** (added — §17, B13).
The values are now `inboundRatePerSimMinute` **100.0** (from 2.0, via 12.0) and
`inboundRateBurst` **50** (from 5, via 15), with a new `--inbound-rate` /
`MULTIVERSE_INBOUND_RATE` knob; `contract-a.md` §18 A40 carries the derivation and the
measurements. The reason it belongs in this document's tunable table and not only in that one:
**the limit was sized against a one-row ring where a slot had one inbound lane, and every
topology decision since has been made here.** §2's grid gave each slot a second inbound lane,
§17's B13 gives it four, and §2.1's degenerate axes make the columns of a 3×2 map carry two
lanes to the same peer. Thirty-five hours of the living deployment measured a median offered
load of 1.19 arrivals per simulated minute per slot against a 2.0 ceiling — 12% of samples
over it, and three of six slots holding a paced backlog pinned at `inboundQueueMax` — which is
`contract-a.md` §7.5's own definition of a limit set too low. **A topology change on this wire
is a pacing change on the other one**, and the next map that grows an axis should re-check this
number before it ships, not after.

**The second raise is the evidence for that sentence.** 12.0 was derived from a *projection* of
two-way traffic, written before B13's reverse lanes ran. Once they ran, the residual paced
backlog did not clear — slot 3 held a `pacedDepth` of 16 under the raised limit — so the owner
raised the default again to **100.0/50** the same day (signed off 2026-08-07). 100.0 abandons
"five times the median" and sizes the limit from the mod-side ceiling instead: A29's ingest
budget of 4 applications per `FixedUpdate` is ~12 000 per simulated minute, **two orders above
it**. The burst is bounded by `inboundQueueMax` (64) rather than by the rate, so the bucket can
never release a full paced queue in one breath. Nothing in the table below changes; the
`OVERLOADED` path simply fires still less often.

**And since the third raise, each sidecar publishes the value it is actually running with**
(added — §18, B16): `inboundRatePerSimMinute` and `inboundRateBurst` are on the peer stats
block of §6.3.1, beside the `pacedDepth` they explain. Two raises in one day is the argument —
a default in a table is not what a rig runs, a `--inbound-rate` on one slot's command line is,
and a `pacedDepth` read against the wrong cap is how a phase of this system's own exit test
came to assert against a number that had moved underneath it twice.

| Interaction with this wire | Effect of the raise |
|---|---|
| `OVERLOADED` backpressure (§6.6, §6.8) | **Less often, not differently.** `inboundQueueMax` (64) is unchanged; a faster drain means the queue reaches it more rarely. Every rule about the refusal, the proof it constitutes, and the re-route it triggers is untouched. |
| The hold clock (§9.3, Risk 9) | **Unchanged, and slightly safer.** The clock already runs only while the destination is **dark**, so a paced backlog at a live peer never accrued hold time. A shorter backlog shortens the window in which a slow peer *looks* like a silent one at all. |
| `forwardRetryMs` / `forwardRetryMaxMs` (§14, B8) | Unchanged. A live destination that is draining faster answers sooner, so the doubling backoff climbs less often. |
| `maxReroutes` (§9.2) | Unchanged. Fewer `OVERLOADED` refusals means fewer re-routes consumed by congestion rather than by darkness. |

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
   state — and, since §16, the name of every species living in it (§6.3.1, B11).
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
   it. M5's per-peer credentials are the precondition for trusting any of it. **The census
   widens the surface without changing the argument** (§16, B11): a species name is
   attacker-chosen text of up to 64 bytes a slot, 32 entries a peer, that lands in a broadcast
   and then in a renderer. The wire's own answer is the shape check and the cap; the
   **renderer's** answer is its own, and a page that interpolates a name into HTML without
   escaping it has a defect this contract cannot fix for it. Named here rather than left to be
   discovered, because raw names are exactly the field an implementer assumes has been
   sanitized upstream — and A36 guarantees it has not.

---

## 14. Reconciliation amendments (`contract-b/3.0`, 2026-08-05)

The Go relay, sidecar and archive were built from §1 to §13 above, in commit `823a70f`. The
implementation resolved four questions this document either left open or answered twice, and
this section makes those resolutions law.

**All four are clarifying.** No field is added, none is removed, no enum value changes
meaning, and no frame changes shape — so `contract-b/3.0` is unchanged, and an implementation
that already matches the code needs no work. What changes is the *document*: two places where
it named a knob it never defined or stated a rule in only one of the two sections that need
it, and two places where it was simply narrower than the truth. `contract-a.md` §15 carries
the matching pair for the other wire, A26 and A27.

**The numbering continues the `B` series** rather than restarting it. B1, B2 and B3 are
`contract-b-m2.md`'s, and §11 item 3 above still cites B1 and B2 by name for a rule it carries
forward — so a fresh `B1` here would collide with a live reference in this same file. The
namespace is the wire's, not the file's.

**The style follows `contract-a.md` §13–§15**, which is what a reader of both contracts will
expect: each amendment names the ambiguity or gap, the resolution, and **which side enforces
it** — the side whose code makes the rule true, and which therefore has to change if the rule
changes. Where an amendment sharpens the body, the body carries an `(amended — §14, Bx)` or
`(added — §14, Bx)` marker at that point.

**B8 was folded in on 2026-08-06, from the slot-6 livelock** (`contract-a.md` §15 A29 is
the matching set for the other wire). It is behavioural, not structural: no field, no enum,
no frame shape — one decode-order rule §6.6 already implied, one retry cadence §9.3 never
bounded, and one new named default in §12 (`forwardRetryMaxMs`). `contract-b/3.0` is
unchanged.

### B4 — `statsBroadcastIntervalMs` is defined, at 5 000 ms (§6.5, §12)

**The gap.** §6.5 requires `PEER_STATUS` "also sent on a `statsBroadcastIntervalMs` timer,
because stats change without the registry changing", and §12 never gave the name a row. A
tunable named in a requirement and absent from the tunables table is a value every
implementer has to invent, and two relays that invent differently age the same status page
differently.

**Resolution.** §12 gains the row. `statsBroadcastIntervalMs`, default **5 000 ms**, owned by
the **relay**.

| Rule | Statement |
|---|---|
| What fires | The relay republishes `PEER_STATUS` and re-sends any `SECTOR_GRANT` whose content changed. It is the same publish path a registry change takes, on a timer instead of on an event. |
| Why 5 000 | It equals `statsIntervalMs`, the cadence at which a sidecar's stats block arrives on `PING` (§6.11). A faster timer republishes a block that has not changed; a slower one publishes state that is already stale on arrival. The two cadences are the same cadence, and tying them is what keeps `statsAsOfMs` meaning what §6.5 says it means. |
| Not a knob | It is a compiled default with **no flag and no environment variable**. An operator has nothing to tune here, and the pair of intervals is easier to keep consistent when only one of them is settable in a test. |
| Interaction with `statusCoalesceMs` | None to reason about: coalescing bounds how *closely* two broadcasts may follow each other (250 ms), and this timer bounds how *far apart* they may drift. |

**Enforced by:** the relay.

### B5 — A held entry keeps retrying, and that retry is the only conduit for the proof (§9.2, §9.3)

**The conflict.** §9.2's rule table, row 4, says a forwarded-then-silent entry should "hold,
then bounce — retry the recorded `destSlot` on the retry cadence". §9.3 then describes the
hold in full — the clock, when it runs, when it stops, where it lives, what happens at the
timeout — and **never mentions the retry**. A reader who builds the hold from §9.3, which is
the section written for that job, builds one that waits in silence. That is the natural thing
to build and it is wrong.

**Resolution.** §9.3 states the rule explicitly, and it is a **MUST**:

| Rule | Statement |
|---|---|
| The retry | A `held` entry keeps re-forwarding its **recorded** `destSlot` on `forwardRetryMs`, while the destination is dark. |
| What the retry does not do | It does not change the handoff state — a `held` entry stays `held` — and it does not reset or pause the accrual. A retry that promoted the entry back to `sent` would restart the very clock §9.3 exists to accrue. |
| Why it is safe | The destination deduplicates on `migrationId` (§6.6 step 1). A delivery that did happen answers as a duplicate; one that did not answers with the relay's statement. |

**Why this is load-bearing, and not a detail.** The relay never volunteers a
`neverForwarded` statement — it mints one **only** in answer to a `MIGRATION_PAYLOAD` it
declined to forward (§5.2, §6.8). So the retry is not one way of collecting the proof, it is
the **only** way. Trace the case the milestone was built for: a sender hands the relay a frame
for a slot that has already gone dark. The write to the relay succeeds, so the entry is
`sent`, then `held` — and the relay's forwarding record holds nothing for that `migrationId`,
because it never wrote the frame onward. The proof exists. It is waiting. Only asking again
fetches it. **Remove the retry and `held → refused → re-route` is unreachable**: every held
entry runs the full 24-hour hold and bounces home, including the large majority that could
have re-routed within a retry interval. Route-around would not degrade — it would be off for
precisely the case it was written for.

**Enforced by:** the sidecar, which must run the retry and the hold accrual on the same entry
concurrently, and must not let either reset the other.

### B6 — A duplicate is re-ACKed only when it is tombstoned (§6.6, §6.7)

**The conflict.** §6.6 step 1 said a dedup hit "is answered `MIGRATION_ACK` immediately and
delivered nothing". §6.7 says the opposite for one of the two cases: a sidecar **MUST NOT**
send `MIGRATION_ACK` when it has merely journaled the payload, and it names exactly one
exception — a `migrationId` that is already **tombstoned**. Both sentences are in this
document and they do not agree.

**Resolution.** **§6.7 is right and §6.6 is aligned to it.** A dedup hit always delivers
nothing; whether it is *answered* depends on the kind of hit:

| The hit | Answer | Why |
|---|---|---|
| Tombstoned — delivered to the mod and ACKed once | `MIGRATION_ACK`, `duplicate: true` | Delivery is proven. Re-stating a proven fact adds nothing and costs nothing. |
| Journaled, not yet delivered to the mod | **nothing** | Delivery is not proven. |
| Journaled, delivered, `MIGRATE_IN_ACK` not yet received | **nothing** | Delivery is not proven *yet*, and the spawn is what proves it (§6.7). |

**Why the narrow rule, in one sentence: an early ACK releases the sender's custody before the
delivery it claims.** The chain (D2) puts custody transfer at the receiving mod's spawn. A
duplicate arriving against a journaled-not-tombstoned entry means the first copy is still in
that journal, still waiting for its mod — and answering it hands the sender permission to
drop its own entry. If the receiver then dies before the spawn, **both** sides have let go of
the organism. That is a loss the chain exists to prevent, and it is a loss with no signal:
nobody logs anything.

**Pacing is what turns this from a race into a window.** Arrival pacing (`contract-a.md`
§7.5 and §15 A20) makes "journaled but not yet delivered" a **long-lived** state — minutes of
simulated time, by design. In M3 that state lasted microseconds and the unconditional re-ACK was a
theoretical defect; under M4 it is a wide, ordinary, everyday window. **This amendment records
a live defect found and fixed in the M4 implementation**, not a hypothetical one.

**The cost of the narrow rule is zero.** The sender does not stall: its retry lands right back
in the same branch, and §9.3's hold clock does not run while the destination is live (Risk 9).
Silence against a live peer is not an orphan, and §9.2 already says so.

**Enforced by:** the receiving sidecar. The sender needs no change — it already retries — and
the relay never sees the distinction.

### B7 — The relay fans out its own non-delivery answers, and a NACK dedups on `migrationId` + `code` (§5.1)

**The gap.** §5.1's fan-out rule says the relay copies every `MIGRATION_PAYLOAD`, `ACK` and
`NACK` **it routes**. A relay-*generated* `MIGRATION_NACK` is not routed — it is minted and
sent straight back to the sender — so a literal reading excludes exactly the frames that
prove a hop reached nobody. The archive would then record a payload with no outcome and no
way to tell a stalled migration from a refused one.

**Resolution — the fan-out set is a superset of the routed set.**

| Frame | Fanned out? | Why |
|---|---|---|
| `MIGRATION_PAYLOAD`, `MIGRATION_ACK`, `MIGRATION_NACK` the relay **routes** | yes | Unchanged from M3. |
| `MIGRATION_NACK` the relay **generates in answer to a `MIGRATION_PAYLOAD`** — `SLOT_VACANT`, `PEER_OFFLINE`, `NOT_FORWARDED` | **yes** | These three carry `neverForwarded` and `relaySessionId` (§5.2, §6.8). They are the only record a subscriber can have of a hop that reached no sidecar, and without them the status page cannot say why a lane is not flowing. |
| `NOT_A_MEMBER` — a payload from a subscriber | no | A **role** error about a connection. No migration was ever in question, and the offending frame was not forwarded to anybody. |
| `PEER_UNKNOWN` — a routed `ACK`/`NACK` whose `destPeer` has gone | no | A **routing** error about a frame the archive already has a copy of, from the fan-out of the original. |

**And the archive's dedup key for a NACK is the pair `migrationId` + `code`**, not
`migrationId` alone. One migration legitimately produces several *different* refusals on its
way to a lane — `PEER_OFFLINE` on the first attempt, then `NOT_FORWARDED` on the retry that
finally collected the proof (B5). Those are two facts about two attempts. Deduplicating them
into one would erase the sequence, and the sequence is the evidence a re-route has to be read
against. A `MIGRATION_PAYLOAD` and a `MIGRATION_ACK` still dedup on `migrationId` alone.

**The key must be built in exactly one place**, and writing this amendment is what found the
reason. The archive had two: the live path composed `migrationId` + `code`, while the restart
replay rebuilt the seen-set from disk under `migrationId` alone, for every record type. The
two keys could never match, so a NACK re-copied after a restart was recorded a second time —
the §5.1 guarantee held within a process and not across one. A duplicate *row*, never a
duplicate organism, and invisible until a restart happened to coincide with a re-forward. The
reconciliation pass collapsed both call sites onto one key function.

**Enforced by:** the relay, for the fan-out; the archive, for the key — and for keeping the
key single-sourced, which is the part that broke.

### B8 — Dedup precedes the decode, and a live destination's retry backs off (§6.6, §9.2, §9.3, §9.4, §12)

*Folded in on 2026-08-06, from the slot-6 livelock. `contract-a.md` §15 A29 carries the
Contract A half of the same incident.*

**The gap.** §6.6 orders the receiver's obligations and puts dedup first, and §9.2/B5 order
the sender to keep re-forwarding because "the retry is free — the destination
deduplicates." Both sentences are true and their composition is not: nothing said dedup
comes before the *decode*, and nothing bounded the retry toward a destination that is live
but not answering. Slot 6 was exactly that destination for seven hours — its paced backlog
pinned at `inboundQueueMax`, its mod livelocked, its relay link healthy — so its sender
re-forwarded every stuck entry every `forwardRetryMs`, and slot 6 paid a full
`MIGRATION_PAYLOAD` decode, multi-MiB `body.bb8` included, for each of 203,735 duplicates
before looking up the `migrationId` that made the whole frame moot. The "free" retry was a
CPU tax on the one machine that had none to spare.

**The resolution, receiver half.** §6.6 step 1 is a decode order, not just a processing
order: extract `migrationId` with a minimal parse, look it up, and answer a hit from the
table **before** decoding the body. Two consequences, both deliberate:

1. A duplicate costs an O(1) lookup and a header parse, whatever its size.
2. A duplicate whose body is malformed is answered as a *duplicate* — silence, or a
   tombstone re-ACK — never `MALFORMED_MESSAGE`. Before B8 the decode ran first, so such a
   frame drew a NACK the table never sanctioned. The organism is already in this journal;
   its custody is the question, and the frame's validity stopped mattering when the first
   copy was journaled. A **new** `migrationId` with the same bad body still fails
   validation exactly as before.

**The resolution, sender half.** The re-forward toward a **live** destination doubles from
`forwardRetryMs` up to `forwardRetryMaxMs` (60 000 ms), per entry, resetting on any handoff
state change or liveness flip. The dark cadence is untouched — B5's argument stands whole:
toward a dark destination the retry is the only conduit the non-delivery proof has, and the
hold clock accrues against it. Toward a live destination no proof is pending — the
destination has the frame and is pacing it toward its mod (Risk 9) — so the retry exists
only to survive a lost frame, and a decaying cadence serves that at a fortieth of the cost.

**What does not move.** `MIGRATION_ACK` still waits for the receiving mod's
`MIGRATE_IN_ACK` (§6.7); a merely-journaled duplicate is still answered with nothing; the
hold clock still runs only while the destination is dark; and no frame, field or enum
changes. One default is added to §12.

**Enforced by:** the receiving sidecar, for the decode order; the sending sidecar, for the
backoff. The relay carries no part of it — it forwards retries as dumbly as ever, which is
why the sender had to learn restraint.

---

## 15. Species-identity amendment (`contract-b/3.1`, 2026-08-07)

The owner ratified **Option A — species identity travels in the migration envelope** on
2026-08-07. A `.bb8` payload carries only `genes.speciesID`, a per-world counter value that
means nothing in another world, and the destination's deserializer looks it up in its **own**
registry anyway — so a migrant silently joins an unrelated local species on a collision, and
founds an unprunable orphan-root species on a miss. `contract-a.md` §16 states the defect in
full and carries the whole design; **this wire's entire job is to carry the answer without
touching it.**

**Two amendments, B9 and B10**, continuing the `B` series for the reason §14 gives — the
namespace is the wire's, not the file's. B9 puts the block on `MIGRATION_PAYLOAD` and states
the opacity rule; B10 records it in the archive and applies §4's version test.

**This set changes the wire, additively**: one OPTIONAL field on one message. No message type,
no enum value, no removal, no type change, no new tunable, no new NACK code, and no change to
routing, custody, dedup, pacing, hashing or the hold. §4's test therefore answers **minor**,
and the identifier moves to `contract-b/3.1` (B10). The style follows §14 and
`contract-a.md` §13–§16: the gap or change, the resolution, and **which side enforces it**.

### B9 — `MIGRATION_PAYLOAD` carries an OPTIONAL species block, opaque end to end (§5, §5.1, §6.6)

**Change.** The envelope gains `species`: `{ genericName, specificName, parentGenericName?,
parentSpecificName? }`, copied out of the origin mod's Contract A `MIGRATE_OUT.species`
(`contract-a.md` §5.3) and handed to the destination mod on `MIGRATE_IN` (`contract-a.md`
§5.7). §6.6 carries the field table and the six rules; this amendment states why they are
what they are.

**Resolution — the block is envelope metadata beside the blob, exactly like `exitEdge`.**

| Question | Answer |
|---|---|
| Who may read it? | The origin mod, which writes it, and the destination mod, which resolves it against its own registry. **Nobody in between.** |
| What do the sidecars do? | Validate its shape and carry it. A malformed block is **stripped and logged**, never NACKed — refusing an organism over a label is the trade `contract-a.md` §9.1 refuses to make. |
| Who repairs a name that fails the shape rules? | **The exporting mod, at the source, and nobody on this wire** (`contract-a.md` §16, A34). The game generates an edge space in about 2% of name halves; the origin mod trims and collapses whitespace before it validates, so the block that reaches a sidecar is already clean. **No rule here changes** — a sidecar still validates and still MUST NOT normalize — and that is the point: this wire can keep taking the clean-name rule literally because the repair happened before the wire. |
| What does the relay do? | Nothing. §5's prohibition list now names `data.species` beside `data.body.bb8` and `data.lineage`. A species name is never a routing input, a filter or an admission-control term. |
| Does the fan-out change? | **No.** §5.1 already copies every routed frame byte-identically, so a subscriber receives the block without a new rule. |
| Does a re-route change it? | **No.** A re-routed frame keeps the same `migrationId`, the same body and the same block; only `destSlot` and the `reroute` annotation move (§6.6, §9.2). |
| Is it in the annex? | **No, and deliberately.** `lineage` holds values this wire *computes* under `genome-hash.md`. A species name is asserted by the origin world, is not hashed, and is excluded from the canonical projection (`genome-hash.md` §4.3) — so the rewrite the destination mod performs on `genes.speciesID` cannot invalidate a hash this wire already computed. Nesting an unverifiable string among computed ones invites an implementer to reconcile it against a hash that was built to ignore it. |

**Why this is D4-consistent and not a widening of it.** D4 keeps the *body* opaque. This block
is not the body — it never came out of the body, and it never goes back into it on this wire.
The one place a name touches a payload is inside a game process, where the destination mod
rewrites `$.genes.speciesID` before restoring (`contract-a.md` §16, A31), and that is a mod
rule in a mod's own contract. Nothing on Contract B parses one more byte than it did before.

**Enforced by:** the source sidecar, for copying without authoring; both sidecars, for
shape-validate-and-strip; the relay, for reading nothing; the receiving sidecar, for handing
the block through verbatim.

### B10 — The archive records the block, and the version moves to `contract-b/3.1` (§4, §10, §10.1, §12)

**Change.** The archive stores the `species` block against the migration record, and §4's
version test is applied to the field B9 added.

**Resolution, the recording half.** §10 carries the rules: recorded verbatim; a **ledger fact,
never a resolution** — the archive does not resolve, merge or rewrite names, because species
resolution happens in exactly one place in this system and it is the importing mod; absent is
absent, never `"unknown"` as a value; and dedup is unchanged, on `migrationId` alone. §10.1
gains **no** page input in M4: a rendering may follow and this document specifies none.
*(Superseded in part — §16, B12: the rendering arrived on 2026-08-07, and it reads the live
census in `stats.species`, not this ledger. Every other sentence in B10 stands, including this
one's substance — the ledger is still not an input to any abundance claim.)*

**Why record it at all, given no page reads it.** The archive's whole purpose is that
migration is where a genome crosses a machine boundary (D11), and until now the ledger could
name only hashes. The name its origin world used is the one label a human reads, and it is
free here — the envelope already passes through. Recording it now means the history exists
when something wants to render it; adding it later would leave every migration before that
date nameless.

**Resolution, the version half.** Apply §4's test:

| Change to Contract B | Kind | Needs a major? |
|---|---|---|
| `MIGRATION_PAYLOAD.species` added | additive OPTIONAL field | no |
| Relay prohibition list names one more field | a restriction restated, no shape | no |
| Archive records a field it already receives | off-wire behaviour | no |
| Message catalogue, enums, codes, tunables, custody rules | **all unchanged** | no |

The identifier is **`contract-b/3.1`**. Every `contract-b/3.x` peer stays compatible with every
other, because compatibility is on the major and the minor is never a rejection reason (§4,
`contract-a.md` §3.1). The two old-peer cases are **not** the same, and the difference is
worth stating: a `contract-b/3.0` **relay** is transparent to this change — it forwards frame
bytes and decodes nothing (§5), so the block crosses it intact — while a `contract-b/3.0`
**sidecar** at either end drops the field on decode, and the destination mod then applies its
absent-block rule (`contract-a.md` §16, A32), which is defined, quiet, and still better than
the behaviour §16 replaces. **No default in §12 changes and none is added.**

**Enforced by:** the archive, for the recording and for not interpreting it; both wire ends,
for sending `"contract-b/3.1"` and comparing only the major.

---

## 16. Species-census amendment (`contract-b/3.2`, 2026-08-07)

The owner ratified **the species census on the live map** on 2026-08-07. `contract-a.md` §17
carries the design: the mod reports its world's active species on every `HEARTBEAT`, as a
**display** census — raw names, two counts per species, sorted, capped at 32. **This wire's
job is to get that census to the operator surface without touching it**, which is the job it
already does for `population`, `lastSave` and every other stat.

**Two amendments, B11 and B12**, continuing the `B` series for the reason §14 gives — the
namespace is the wire's, not the file's. B11 puts the census in the peer stats block; B12
amends §10.1, which until now said the archive's species names were not a page input, and
applies §4's version test.

**This set changes the wire, additively**: two OPTIONAL fields inside one existing block, on
the three carriers that block already has. No message type, no enum value, no removal, no type
change, no new NACK code, and no change to routing, custody, dedup, pacing, hashing, the hold
or the fan-out. One shared default is named in §12. §4's test therefore answers **minor**, and
the identifier moves to `contract-b/3.2` (B12). The style follows §14, §15 and
`contract-a.md` §13–§17: the gap or change, the resolution, and **which side enforces it**.

### B11 — The peer stats block carries the species census, copied and republished blind (§6.3.1, §6.5, §6.11, §12, §13)

**Change.** The stats block of §6.3.1 gains `species` — the array `contract-a.md` §5.2 defines,
`{ genericName, specificName, bibites, eggs }` per entry — and the `truncated` boolean that
qualifies it. Both OPTIONAL. The block's three carriers are unchanged: a sidecar sends it on
`SECTOR_CLAIM` (§6.3) and on `PING` (§6.11), and the relay republishes the latest it holds in
`PEER_STATUS` (§6.5).

**Resolution.**

| Rule | Statement |
|---|---|
| Copy, never author | The sidecar copies the census out of the last `HEARTBEAT` it received. It **MUST NOT** synthesize one, re-sort one, merge two entries, fill a missing count, or derive a census from anything else it holds — not from its journal, not from its genome cache, not from the `MIGRATION_PAYLOAD.species` blocks that pass through it. Those describe migrants; this describes a population. |
| Validated once, on the other wire | The shape rules and the strip rules live in `contract-a.md` §5.2, where the census enters the system: a malformed **entry** is stripped and `truncated` is set, a malformed **field** is dropped whole, and neither is ever a NACK or a close. What reaches this wire has already passed that check, so this wire adds no second validator — it adds a **bound**: an over-long array is trimmed to `speciesCensusMax` with `truncated: true` (§12). |
| Raw names, and no party here may repair one | Each half is carried byte for byte, edge whitespace and all. **No sidecar, relay, archive or page may trim, collapse, case-fold, normalize or re-case a census name** — the reason is `contract-a.md` §17, A36's, and it is the opposite of §15 B9's lane on purpose. |
| The relay's rule is already written | §6.3.1's *"the relay does not interpret any of it"* and §6.11's *"it never routes, refuses, schedules or filters on a stat"* cover the census exactly as they cover `population`. The relay stores the block, stamps `statsAsOfMs`, and republishes. **B11 adds no relay behaviour**, and a relay that treated a species name as a routing, filtering or admission-control term would be violating a rule this document has had since M4, not a new one. |
| One **SHOULD**, for the next field after this one | A relay **SHOULD** store the stats block as the bytes it arrived as rather than re-encoding it from a typed model, so a stat a newer sidecar sends survives an older relay. `contract-b/3.1` relays that re-encode simply drop the census, which degrades to unknown (B12) — correct, and avoidable next time. |
| Absence is a value, and there are two of them | An absent `species` means **unknown**: no mod, a mod older than `contract-a/2.2`, or no heartbeat carrying one yet. A present `[]` means a reporting mod with nothing alive. §10.1 renders them differently; §6.3's two claim examples show both, and §6.5's broadcast shows an absent one beside four present ones. |
| Staleness is inherited, not re-invented | `statsAsOfMs` already ages the whole block (§6.5), and `statsStaleMs` (30 000) already decides when it stops being state (§10.1). A census needs no clock of its own, and giving it one would let a page show a fresh species list beside a stale population from the same frame. |
| Bounded by construction | `speciesCensusMax` (32) caps the array, ~3 KB caps a full census, and a six-slot `PEER_STATUS` therefore grows by at most ~20 KB (§6.3.1). The cap is a wire constant shared with `contract-a.md` §10, not a display preference, and it is what keeps this addition free of a new backpressure question. |

**Why the stats block and not a new message or a new subscription.** The block exists to
describe six worlds — two of them on a machine the archive cannot read a file from — without
anything reading anything else's memory (§6.3.1, Risk 4). "Which species live there" is that
same question with a different noun. A second channel would need its own cadence, its own
staleness stamp and its own place in the fan-out, and it would let a page show a species list
that disagrees with the population printed beside it because the two arrived in different
frames. One block, one timestamp, one truth.

**What this does not touch.** `MIGRATION_PAYLOAD` is unchanged — the census does not ride on a
migration and the migration block does not ride on a stat. The two carry species names for
different reasons, under different name rules, on different cadences, and §15 B9's six rules
are unaffected in every particular.

**Enforced by:** the sidecar, for copying the last census without authoring, re-sorting or
repairing it; the relay, for republishing it as blindly as it republishes a population — a
rule §6.3.1 already states; the archive and its page, for rendering it under §10.1 and for
escaping it as untrusted text (§13 item 7).

### B12 — The page renders the census, the ledger is still not abundance, and the version moves to `contract-b/3.2` (§4, §10, §10.1, §12)

**The contradiction this closes, stated plainly.** §15's B10 wrote into §10.1 that *"the
species names the archive now records are not an input to this page in M4"*, and into §10 the
row *"Not an M4 page input"*. Both sentences were true when they were written and one of them
is now false: **the page has a species view.** Leaving them would put this document in
conflict with the running system in the one section whose entire job is to say what the page
may claim.

**Resolution, the rendering half.** The page's species view comes from `stats.species` in
`PEER_STATUS` (§6.3.1, B11), and from nothing else. §10.1 gains two rules — the census obeys
every existing one, and the census and the ledger answer different questions — and §10's row
is amended to say which of the two is abundance.

| Question | Answer |
|---|---|
| Where does the page's species view come from? | The **census**, in the stats block of every `PEER_STATUS` the archive already receives as a subscriber. §10.1's first rule is unchanged and unweakened: one source, no polling, and nothing on the migration path ever waits for a reader. |
| Is the archive's ledger of `MIGRATION_PAYLOAD.species` an input? | **No, and B10's substance stands.** The ledger records what **crossed**. A database built from migrations holds migrants and their ancestors, never the resident population of a peer (D11), so an abundance claim drawn from it would be a plausible-looking wrong number. |
| May the page show ledger names at all? | Yes — as what they are: which species crossed which lane, and when. Labelled as history, never as population, and never summed into a census. |
| May the page join the two? | Only with care, and the care is named: the ledger's copy is **normalized at the source** (`contract-a.md` §16, A34) and the census's is **raw** (`contract-a.md` §17, A36), so the same species can legitimately appear as `Izus` and `Izus `. Normalize **for the comparison only**, rewrite neither, and say which side a displayed spelling came from. |
| What does the page do with a missing census? | **Unknown.** Never zero, never an empty list, never the last census seen without its age. That is §10.1's own rule, applied to a new field rather than bent for it. |

**Resolution, the version half.** Apply §4's test:

| Change to Contract B | Kind | Needs a major? |
|---|---|---|
| `stats.species` added | additive OPTIONAL field | no |
| `stats.truncated` added | additive OPTIONAL field | no |
| `speciesCensusMax` named in §12 | a shared default for a new field | no |
| §10.1 gains two rules; §10's B10 row is amended | page-input rules, off the wire entirely | no |
| Message catalogue, enums, codes, custody, routing, fan-out, hashing | **all unchanged** | no |

The identifier is **`contract-b/3.2`**. Every `contract-b/3.x` peer stays compatible with every
other, because compatibility is on the major and the minor is never a rejection reason (§4,
`contract-a.md` §3.1). The old-peer cases differ, and the difference is worth stating: a
`contract-b/3.1` **relay** that stores the stats block as received carries the census intact
and needs no change at all, while one that re-encodes from a typed model drops it; a
`contract-b/3.1` **sidecar** never sends one; and a mod older than `contract-a/2.2` produces
none to send. **All three degrade to the same place — unknown — and none of them produces a
census that is wrong.** That is the property the whole design is arranged around, and it is
the same standard §10.1 already holds every other stat to.

**Enforced by:** the archive, for rendering the census under §10.1 and for keeping its
migration ledger out of every abundance claim; both wire ends, for sending
`"contract-b/3.2"` and comparing only the major.

---

## 17. Two-way lanes and the hop feed (`contract-b/3.3`, 2026-08-07)

The owner ratified **D17 (two-way lanes)**, **D18 (migration exclusion)**, **D19 (live hop
animation)** and **D20 (the pacing raise)** on 2026-08-07, against the running M4 deployment
(`system_decomposition.md`). `contract-a.md` §18 carries the mod-and-sidecar half: every
declared edge becomes both an export edge and an entry edge, with **no change to that wire at
all**, because its fields have accepted four edges since A18 and its corner rule since A26.

**This wire is where the change is real.** A two-way map is a *routing* fact, and routing is
this document's job: the relay has to compute two more effective-neighbour walks and name them
in the grant. **Three amendments, B13 to B15**, continuing the `B` series for the reason §14
gives — the namespace is the wire's, not the file's. B13 adds the reverse lanes; B14 records
the hop feed the page animates; B15 applies §4's version test.

**This set changes the wire, additively**: two new keys in one existing object, keyed by an
enum whose four values have been legal since M2. No message type, no field removal, no type
change, no enum value added or removed, no new NACK code, and no change to custody, dedup,
the hold, the fan-out, hashing or admission control. §4's test therefore answers **minor**,
and the identifier moves to `contract-b/3.3` (B15). The style follows §14, §15 and §16: the
gap or change, the resolution, and **which side enforces it**.

### B13 — The grant names four effective neighbours, and the walk runs in both directions (§2, §2.1, §6.4, §8, §12)

**Change.** §8's walk produced one effective neighbour per **export** edge, and D13 made that
two. D17 makes every edge an export edge, so the walk runs four times: east and north as
before, plus **west and south, which are the same two walks with the step negated**.

**Resolution.**

| Rule | Statement |
|---|---|
| The walks | `effective(W)` = first deliverable slot at `(col−1, row), (col−2, row), …` mod width. `effective(S)` = the same down the column. §8 carries both. |
| `deliverable()` | **Unchanged**, all six conditions, including `slot ≠ me`. One filter, four walks. |
| `SECTOR_GRANT.neighbours` | Gains the `"W"` and `"S"` keys, with the identical sub-object every other key carries. A key is present when that edge has a deliverable target and **absent** when it does not, which is what closes it with `no_peer` — unchanged from §6.4. |
| Which keys the relay emits | One per edge the sidecar **declared** in `SECTOR_CLAIM.exportEdges` and found a target for. The relay never invents an edge the sidecar did not declare, so a two-edge sidecar's grant is byte-identical to today's. |
| Which keys a sidecar reads | Those it declared. It **MUST** ignore a key for an undeclared edge and **MUST NOT** treat an absent key as an error — the same forward-compatible rule `contract-a.md` §5.4 applies to `EDGE_STATUS`. |
| Skip lists | **Per walk.** `E` and `W` traverse one row from opposite ends, meet its dark slots in a different order, and stop at the first live one, so their `skipped` lists routinely differ even when they name the same target. |
| The close mapping (§8) | **Unchanged and per edge.** A closed `W` reports the aggregate of the `W` walk's own skip list, by the same six-row table. |
| The ripple (§8) | **Symmetric now.** Every neighbour of a dark slot both pointed at it and was pointed at by it, so the "told nothing" case is gone. Mechanism unchanged; the set it applies to doubled. `statusCoalesceMs` already bounds the burst. |
| `PEER_STATUS` | **Not touched.** It publishes the registry and the structural row-major order; lanes have always been *derived* from it, and two more derivations need no field. Any client can reproduce all four walks for display, exactly as §10.1 requires it to for two. |
| `MIGRATION_PAYLOAD.exitEdge` | May now be `"W"` or `"S"`. Existing enum value, carried for the record. **The relay has never routed on it** — routing is on `destSlot` — so nothing in §5 changes. |

**What did not change, checked rather than asserted.** D17 retires §2's *out and in are
different doors*, and the honest question is what depended on it. The answer is **nothing on
this wire**, and each load-bearing mechanism keys on something else:

| Mechanism | Keys on | Affected? |
|---|---|---|
| Routing (§5) | `destSlot` | No. The relay reads an address, never an edge. |
| Custody and dedup (§5.1, §6.6, §9.1) | `migrationId` | No. |
| The non-delivery proof (§5.2) | the forwarding record for a `migrationId` | No. |
| The bounded hold (§9.3) | destination **darkness**, accrued | No. |
| Bounce-back (§9.4) | the origin's own `exitEdge`, returned home | No — and `contract-a.md` §18 A38 confirms the mod's side: a bounce arrives moving outward, inset past the band, covered by the immunity window, exactly as it already was on `E` and `N`. |
| Re-route (§9.2) | `maxReroutes`, the proof, the lane | No, except that a re-route now has up to four lanes to choose from instead of two. |

The one thing that genuinely changes is **traffic**, and §2.1 says where: on an axis of length
2 the forward and reverse walks name the same slot, so the columns of a 3×2 map become
two-lane pairs. That is why D20's pacing raise ships in the same wave (§12,
`contract-a.md` §18 A40), and why **3×3 is the map that makes both axes honest** under two-way
lanes.

**Enforced by:** the **relay**, for the two new walks, for emitting only declared keys, and for
the symmetric ripple; the **sidecar**, for declaring its edges, for reading only the keys it
declared, and for mapping each to its own `EDGE_STATUS` entry (§8); the **archive and any
client**, for reproducing four walks instead of two when it renders the map (§10.1).

### B14 — The archive keeps a bounded feed of recent hops, and the page animates the species glyph (§10, §10.1, §15 B10, §16 B12)

*D19. Archive-internal and page-only. **No wire field, no message, no relay behaviour.***

**Change.** The archive already records a `species` block against every migration (§10, B10)
and already counts per-lane hops (§10.1). It now also keeps a **bounded, in-memory feed of
recent hops** — lane, species names, timestamp — and serves it beside the status view, so the
page can animate that species' glyph travelling along the lane's arrow.

**Resolution.**

| Rule | Statement |
|---|---|
| Source | The `MIGRATION_PAYLOAD` copies the archive already receives as a subscriber (§5.1). **§10.1's first rule is unchanged and unweakened**: one source, no polling, and nothing on the migration path ever waits for a reader. |
| Bounded twice | By **time** (~60 s) **and** by **count**. Both, not either. The status view is serialized verbatim into the durable metrics file once a minute, so an unbounded array would be written to disk every minute forever. |
| It is ledger, not census | The feed answers *what crossed*, and B12's rules apply unchanged: it is labelled as history, it is **never summed** into an abundance claim, and it is not joined to the census without normalizing for the comparison only (`contract-a.md` §16 A34 versus §17 A36). |
| An absent species block | Renders as the **neutral glyph** — never a guessed name, never omitted, never "unknown" as a species *value*. That is §10.1's unknown rule applied to a new view rather than bent for it. |
| Names are still untrusted text | §13 item 7's escaping obligation applies, and it applies to a name that now reaches a **new** part of the page. A census name and a ledger name are equally untrusted. |
| Not a wire feature | No sidecar, relay or peer learns that the feed exists. A page that cannot render it degrades to the ambient per-lane pulse it already had. |

**Why this needs saying in a wire contract at all.** Because §10.1 is the section whose entire
job is to state what the page may claim, and B12 had to be written precisely because a
previous sentence there went stale when the page grew a species view. The same trap is open
here: a feed of species names moving along lanes is the most abundance-looking thing the page
has ever shown, and it is **not abundance**. A database built from migrations holds migrants
and their ancestors, never a resident population (D11). The glyph on the lane says *this one
crossed*; the glyphs in the cell say *these live here*; and they are two different facts from
two different sources that happen to be drawn with the same shape.

**Enforced by:** the **archive**, for bounding the feed and keeping it out of every abundance
claim; the **page**, for the neutral glyph, the labelling and the escaping.

### B15 — The identifier moves to `contract-b/3.3` (§4, §6.4, §12)

**Change.** §4 and `contract-a.md` §3.1: additive changes raise the **minor**; field removal,
type changes and enum-value removal require a **major**.

**Resolution.** Apply the test, item by item:

| Change to Contract B | Kind | Needs a major? |
|---|---|---|
| `SECTOR_GRANT.neighbours` gains `"W"` and `"S"` keys | additive data in an existing enum-keyed map | no |
| `MIGRATION_PAYLOAD.exitEdge` may be `"W"` or `"S"` | **existing** enum value — the enum has held four since M2 | no |
| §8 gains two walks; §2 and §2.1 gain the reverse lanes | routing computation, no wire shape | no |
| §8's ripple becomes symmetric | relay behaviour on existing frames | no |
| §10.1 gains the recent-hops rule (B14) | a page-input rule, off the wire entirely | no |
| §12 names the raised pacing defaults | defaults owned by `contract-a.md` §10 | no |
| Message catalogue, enums, codes, custody, dedup, routing inputs, fan-out, hashing, the hold | **all unchanged** | no |

The identifier is **`contract-b/3.3`**. Every `contract-b/3.x` peer stays compatible with every
other, because compatibility is on the major and the minor is never a rejection reason (§4,
`contract-a.md` §3.1).

**What a mixed rig does, honestly. Every degradation lands on the one-way map, and none lands
on a lost or misrouted organism:**

1. A `contract-b/3.2` **relay** with a two-way sidecar never computes the reverse walks, so no
   `W` or `S` key reaches the grant. Those two edges close with `no_peer` (§8), the mod is told
   so by `EDGE_STATUS`, and the map runs exactly as it does today. **The sidecar needs no
   special case**: an absent key has always been how an edge closes.
2. A `contract-b/3.3` **relay** with a `contract-b/3.2` sidecar receives an `exportEdges` of
   two, emits two keys, and the peer is a two-edge world on a two-way map. Legal: it receives
   from all four sides and exports to two.
3. A **mod** older than the two-way build declares two edges and produces the same result
   through its own sidecar, with no frame on this wire differing at all.
4. **`exitEdge: "W"` reaching a `contract-b/3.2` relay routes correctly**, because the relay
   routes on `destSlot` and has never read `exitEdge`. An older **archive** records the value
   verbatim, as it records every edge value.

**The version is doing real work this time, and it is worth naming the contrast.**
`contract-a.md` §18 A41 takes **no** bump, because a four-edge mod is already legal against a
`contract-a/2.2` sidecar and claiming otherwise would be false. Here the opposite holds: a
two-way map **cannot** work against a relay that does not compute the reverse walks, and the
minor is the honest statement of which relays do. A peer detects the capability the way §3.1
requires — **by the presence of the `W` and `S` keys in its grant, never by arithmetic on the
minor.**

**Enforced by:** both wire ends, for sending `"contract-b/3.3"` and comparing only the major;
the relay, for being the side whose capability the minor describes.

---

## 18. The pacing and speed readout (`contract-b/3.4`, 2026-08-07)

The owner ratified **the pacing and speed readout on the live map** on 2026-08-07, against the
running M4 deployment. It is a small amendment with one honest cause: `pacedDepth` has been on
the peer stats block since M4 and **has never been readable**. A queue is only deep against the
cap it is queued behind, that cap is counted in **simulated** minutes, and neither the cap nor
the speed that spends it was ever published. In the same day D20 moved the shipped cap twice —
2.0 to 12.0, then 12.0 to 100.0 (`contract-a.md` §18, A40) — which settles the question of
whether a reader may assume it. It may not.

**Two amendments, B16 and B17**, continuing the `B` series for the reason §14 gives. B16 puts
the three settings on the wire; B17 states what the page does with them, records the ambient
lane pulse's removal, and applies §4's version test.

**This set changes the wire, additively**: three OPTIONAL fields on one existing object. No
message type, no field removal, no type change, no enum value added or removed, no new NACK
code, and no change to custody, dedup, the hold, the fan-out, hashing, routing or admission
control. §4's test therefore answers **minor**, and the identifier moves to `contract-b/3.4`
(B17). `contract-a.md` takes **no** bump: not one field of that wire changes, and
`HEARTBEAT.timeScale` has been mandatory there since `contract-a/2.0`.

### B16 — The stats block carries the world's speed and the sidecar's pacing settings (§6.3.1, §6.5, §6.11, §10.1)

**Gap.** §6.3.1 carries `pacedDepth` — "inbound entries waiting on the delivery rate limit" —
and, since M4, the sentence "a depth that never falls names a limit set too low". **The limit
is not on the block.** Neither is the world's `timeScale`, which is the clock that limit is
counted against. An operator reading `pacedDepth: 12` off the live map cannot tell a world
that is nine seconds behind from one that is six minutes behind, and the analyses that have
needed the number have had to divide by a `timeScale` recovered from somewhere else.

**Resolution.**

| Rule | Statement |
|---|---|
| `timeScale` | Copied **verbatim** from the last `HEARTBEAT.timeScale` (`contract-a.md` §5.2). The sidecar does not compute it, smooth it, or infer it from `simulatedTime` deltas. |
| `0` is a reading | A stopped world reports `timeScale: 0`, and that is a **fact about the world**, not a missing value. Absent is the missing value. A reader that folds the two together loses the ability to say "this world is paused", which is the single most common reason a slot's numbers stop moving. |
| `inboundRatePerSimMinute`, `inboundRateBurst` | The sidecar's **own configuration** (`contract-a.md` §7.5, §18 A40), published as settings. The sidecar always knows them, so a peer that omits them is a peer whose build predates this amendment — there is no third case. |
| Never the default | A reader **MUST NOT** substitute the shipped default for an absent cap. The shipped value has been 2.0, then 12.0, then 100.0, all within the life of this deployment, and a rig runs whatever each peer was launched with. |
| The relay | **Unchanged, and that is the point.** §16's B11 already asked a relay to store the stats block as the bytes it arrived as, "for the next field after this one". These are that field. A `contract-b/3.3` relay carries all three without knowing they exist. |
| Not a routing input | Three more numbers copied into a broadcast the relay was already sending. Nothing routes, schedules, refuses or filters on any of them, exactly as nothing does on `population`. |
| Size | Three floats. The block's bound is still the census (§6.3.1). |

**Enforced by:** the **sidecar**, for copying `timeScale` rather than authoring it and for
publishing the configuration it actually runs with; the **relay**, for carrying what it does
not understand; the **archive and any client**, for rendering an absent value as unknown.

### B17 — The page shows speed and pace, what moves on the map is evidence, and the version moves to `contract-b/3.4` (§4, §10.1, §12)

**Change.** Three things, one of them a removal.

**Resolution.**

| Rule | Statement |
|---|---|
| Speed per world | Each world's cell states its `timeScale`. A rig whose worlds run at different speeds is normal and was previously invisible: arrivals are paced on the **receiving** world's clock, so two neighbours at ×1 and ×100 experience the same lane completely differently. |
| Pace per world | Each world states `pacedDepth` over `inboundRatePerSimMinute`, in that order, with the unit named as a **simulated** minute somewhere the reader can reach. Queued-over-cap is the pair; either half unknown renders as unknown **in place**, so a peer that publishes a depth and no cap still shows the depth. |
| Unknown, out loud | An absent cap renders as a marked unknown — not a blank, not a zero, and above all not the default. This is §10.1's third rule with no exception carved for a number that "probably" has not changed. |
| The ambient lane pulse is removed | The map no longer walks a decorative dot down each lane at that lane's measured rate. The rate stays as the lane's own numeric label, which is what a reader can actually compare. **What moves on the map is now always a migration the archive was copied on** (§17, B14) — travelling under motion, still-and-fading under reduced motion, and the destination cell's arrival flash either way. |
| Still a page decision | No field, message, component or wire behaviour is added or removed by any of it. It is recorded here because §10.1 is where what the page may claim is written down, and both "this world runs at ×5" and "everything moving is real" are claims. |

**Version.** Apply §4's test:

| Change to Contract B | Kind | Needs a major? |
|---|---|---|
| Stats block gains `timeScale` | additive OPTIONAL field | no |
| Stats block gains `inboundRatePerSimMinute`, `inboundRateBurst` | additive OPTIONAL fields | no |
| §10.1 gains the speed-and-pace rule and the moving-glyph rule | page-input rules, off the wire | no |
| Message catalogue, enums, codes, custody, dedup, routing inputs, fan-out, hashing, the hold | **all unchanged** | no |

The identifier is **`contract-b/3.4`**. Every `contract-b/3.x` peer stays compatible with every
other, because compatibility is on the major and the minor is never a rejection reason (§4,
`contract-a.md` §3.1).

**What a mixed rig does, honestly.** This is the state the deployment is actually in, and the
degradation is per slot rather than per map:

1. A `contract-b/3.4` **sidecar** against a `contract-b/3.3` **relay** works unchanged: the
   relay carries the three fields it does not know (B11's SHOULD, implemented), and the archive
   reads them off the block it receives.
2. A `contract-b/3.3` **sidecar** — the far end, until its bundle is refreshed — publishes
   neither cap nor speed. Its cell shows `×?` and an unknown cap beside a **known**
   `pacedDepth`, which is exactly the honest split: the depth is real, the scale is not there.
3. A **relay** that re-encodes the block from a typed model instead of carrying it drops all
   three, and every slot reads unknown. That is why B11 wrote the SHOULD down before there was
   a field that needed it.
4. No path produces a wrong number, because there is no default anywhere in the chain to
   produce it from.

**Enforced by:** the **page and `ringstat`**, for rendering unknown rather than a default and
for keeping `timeScale: 0` distinct from absence; the **archive**, for carrying the three
fields through `StatusView` untouched; both wire ends, for sending `"contract-b/3.4"` and
comparing only the major.
