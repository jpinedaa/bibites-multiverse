# Contract A — Mod ↔ Sidecar Wire Specification

**Version:** `contract-a/1.1`
**Amended:** 2026-08-02, amendment set `contract-a/1 + A1–A10` (§13). Both implementations
exist, and each side resolved an ambiguity locally before the other could see it; §13
makes every resolution law. Each place in the body that was wrong or under-specified now
carries an `(amended — §13, Ax)` marker. **§13 wins over the body wherever the two
disagree.** The amendments are additive clarifications, not wire changes: no field, type
or enum value changed, so that set kept the protocol identifier at `contract-a/1` (§3.1).
**Amended:** 2026-08-03, amendment set `contract-a/1.1 + A11–A17` (§14), from the ratified
decisions D8–D11 and the work order in `m3_considerations.md`, *Contract Changes Needed*.
This set **does** change the wire — additively — so the protocol identifier moves to
`contract-a/1.1` (§3.1, §14 A16). Affected body text carries an `(amended — §14, Ax)` or
`(added — §14, Ax)` marker, and **§14 wins over the body and over §13 wherever they
disagree.**
**Status:** implementation-ready for M3. Derived from the ratified decisions D1–D11 in
`system_decomposition.md`, the runtime facts in `m1_findings.md`, the world-geometry and
entry-position research in `m2_findings.md`, and the ring, containment and lineage designs
in `m3_considerations.md`.

This document is the complete interface between `bibites-mod` (C#, in-process with The
Bibites) and `multiverse-sidecar` (Go, a separate process on the same machine). It is
written so a Go implementer and a C# implementer can each build their side without
talking to each other. Where the two sides must agree on a formula, the formula is
written out. Where a value is a tunable, the default is given and the owning side is
named.

The key words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, **REQUIRED**,
**RECOMMENDED**, **OPTIONAL** are used as defined in RFC 2119.

---

## 1. Scope and design constraints

Contract A is the **only** interface the mod knows about. The mod never touches the
network, never learns topology, and never learns a destination. It reports *where an
organism left its own map* and the sidecar decides *where that goes*.

Eight ratified constraints shape everything below. The first five are M2's; the last three
arrived with M3 (added — §14, A11, A12, A13):

| Decision | What it forces on this contract |
|---|---|
| **D2** — durable custody, at-most-once | `migrationId` is the idempotency key. `MIGRATE_OUT_ACK` is emitted only after a durable journal write. Both sides deduplicate. Loss is preferred over duplication. |
| **D4** — the bb8 body is opaque to the mod | The organism payload travels as a **JSON string**, not a nested object, plus a `gameVersion` tag. The mod never parses it. All structural validation is sidecar-side, in `bb8-schema`, in both directions. |
| **D3** — map-edge borders | The mod reports `exitEdge` + `exitPosition` + `velocity` + `heading`. It never reports a destination. The sidecar reports `entryEdge` + `entryPosition`; it never reports absolute world coordinates. |
| **D5** — no global clock | Every timestamp in this contract is informational. No side makes a correctness decision from another side's clock. |
| **D7** — Go sidecar | The sidecar is the WebSocket **server**. The mod is the **client**. A player starts one static binary; the game finds it. |
| **D8** — ring topology (M3) | The mod has exactly **one export edge** (east) and one **passive entry edge** (west). `EDGE_STATUS` governs the export edge only; the entry edge always accepts (§14, A11). The mod still learns no topology — not its slot, not its neighbour. |
| **D10** — containment by the vanilla wrap (M3) | The mod **MUST NOT** disable `worldWrapping`. Export capture reaches **outside** the playable square, and every capture needs an outward velocity (§14, A13). |
| **D11** — lineage annex (M3) | `MIGRATE_OUT` carries the migrant's parent entity IDs and one **opaque** serialized blob per living parent. The sidecar hashes them; the mod never does, because D4 stands (§14, A12). |

---

## 2. Transport

| Property | Value |
|---|---|
| Protocol | WebSocket (RFC 6455) over plain HTTP |
| URL | `ws://127.0.0.1:{port}/contract-a/v1` |
| Default port | `8787` |
| Bind address | `127.0.0.1` only. The sidecar **MUST NOT** bind `0.0.0.0`. |
| Frame type | Text frames. One JSON object per frame. No batching, no newline framing. |
| Encoding | UTF-8, no BOM |
| Compression | Not used in M2. `permessage-deflate` **MUST NOT** be negotiated. |
| Subprotocol | None requested, none required |
| Max frame size | 8 MiB (`maxFrameBytes`), enforced by both sides |
| Concurrency | The sidecar accepts **at most one** mod connection at a time |
| Authentication | None, in M2 **and in M3** (amended — §14, A17). The token M3 adds lives on Contract B, not here — see §12, open item 1 |

The mod connects only while a simulation world is loaded. It connects after the world is
ready and closes with code `1000` when the world unloads or the game quits. The mod
**MUST NOT** hold a connection open on the main menu, and **MUST NOT** dial from it
(amended — §13, A1).

If a second mod connection arrives while one is live, the sidecar **MUST** close the
older connection with code `4006` and serve the newer one. This makes a stale connection
from a crashed-and-restarted game self-healing.

### 2.1 WebSocket close codes

Both sides **MUST** implement this table. A close is not an error report for a single
message — it terminates the session.

| Code | Name | Sent by | Meaning and required reaction |
|---|---|---|---|
| `1000` | `NORMAL` | either | Clean shutdown (world unload, game quit, sidecar stop). The mod reconnects only when a world loads again. |
| `1009` | `TOO_BIG` | either | A frame exceeded `maxFrameBytes`. Emitted by stock WebSocket libraries. Treat as `4003`. |
| `4000` | `PROTOCOL_UNSUPPORTED` | sidecar | The envelope's `protocol` major version is not supported. The mod **MUST NOT** reconnect until it is restarted or reconfigured. It logs one loud error. |
| `4001` | `SLOT_MISMATCH` | sidecar | `CONFIG_UPDATE.ringSlot` disagrees with the ring slot the sidecar holds. A mis-wired rig. The mod **MUST NOT** reconnect automatically. Named `SECTOR_MISMATCH` in M2, over the retired `{x, y}` sector; the code and the behaviour are unchanged (amended — §14, A14). |
| `4002` | `GAME_VERSION_UNSUPPORTED` | sidecar | `bb8-schema` has no support for `CONFIG_UPDATE.gameVersion`. The mod **MUST NOT** reconnect automatically. |
| `4003` | `MALFORMED_FRAME` | either | A frame was not valid JSON, or the envelope was missing a REQUIRED field. The mod reconnects with backoff. |
| `4004` | `HEARTBEAT_TIMEOUT` | sidecar | No `HEARTBEAT` within `heartbeatTimeoutMs`. See §8. The mod reconnects with backoff. |
| `4005` | `SHUTTING_DOWN` | either | The sender is draining. The mod reconnects with backoff. |
| `4006` | `REPLACED` | sidecar | A newer mod connection took over. The old connection **MUST NOT** reconnect. |

The close reason string is free text for humans. No side parses it.

---

## 3. The envelope

Every frame, in both directions, is a JSON object with exactly this shape:

```json
{
  "protocol": "contract-a/1.1",
  "type": "MIGRATE_OUT",
  "messageId": "b7d1e0c4-9f2a-4c31-8b6d-2e0a41f5c7a9",
  "sentAt": 1785693600123,
  "data": { }
}
```

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `protocol` | string | yes | Protocol identifier, major and minor version: `"contract-a/<major>.<minor>"`. This release is `"contract-a/1.1"`. A value with no `.` means minor `0`, so the M2 string `"contract-a/1"` reads as major 1, minor 0 (amended — §14, A16). |
| `type` | string | yes | The message discriminator. One of the nine names in §5. Uppercase, `A–Z` and `_` only. |
| `messageId` | string | yes | UUID v4, lowercase, hyphenated, 36 characters. Unique per frame. Used **only** for log correlation. It is **not** an idempotency key. |
| `sentAt` | number (int64) | yes | Unix milliseconds on the sender's wall clock. Informational only (D5). No side compares it against its own clock to make a decision. |
| `data` | object | yes | The type-specific body. Always present, `{}` when the type carries no fields. Never `null`. |

### 3.1 Protocol version rules

- The version segment is `<major>.<minor>` (amended — §14, A16). Parse it by splitting the
  string at the **last `/`**, then splitting the remainder at the **first `.`**. A missing
  `.` means minor `0`. Both parts are decimal integers without leading zeros.
- Two peers are compatible when the **major** part of `protocol` is equal. **The minor part
  is never a reason to reject a frame or to close a connection.**
- A receiver that reads a different major version **MUST** close with `4000` and process
  nothing from that frame.
- Within one major version, changes are **additive fields only**, and each such change
  raises the **minor**. Field removal, type changes, and enum-value removal require a major
  bump. Adding an enum *value* is additive; removing one is not.
- The minor is a **capability statement, not a negotiation**. A sender **MUST NOT** require
  a field that a lower minor did not define; a receiver detects a feature by the presence
  of its field, never by arithmetic on the minor. The minor exists so a log line and a
  bug report say which shape was on the wire.
- The URL path stays major-scoped: `/contract-a/v1` serves every `contract-a/1.x`.
- Both sides **MUST** ignore unknown fields inside `data` and inside the envelope. This
  is what makes additive changes safe.
- A receiver that reads an unknown `type` **MUST** ignore that frame and log one warning.
  It **MUST NOT** close the connection. An unknown type is a forward-compatible addition,
  not a fault.

### 3.2 Frame validity

A frame is malformed when it is not a JSON object, when `protocol`, `type`, `messageId`,
`sentAt` or `data` is missing, or when any of those has the wrong JSON type. A malformed
frame **MUST** cause a close with `4003`. Never guess a field.

A frame that is well-formed but whose `data` fails validation is **not** a malformed
frame. It is answered with the matching NACK, and the connection stays open. See §9.

---

## 4. Common types and conventions

### 4.1 Scalar types

| Name | JSON | Rules |
|---|---|---|
| `uuid` | string | UUID v4, lowercase, hyphenated, 36 characters. Emit lowercase. Compare case-insensitively. |
| `entityId` | number | **Signed 32-bit integer.** `BibiteBody.id.id` is `Random.Range(int.MinValue, int.MaxValue)` and is very often **negative** (`m1_findings.md` §4.1). Go implementers **MUST** use `int32`, never `uint32`. `0` never appears — it is the game's "unassigned" sentinel. |
| `timestampMs` | number | Unix milliseconds, int64. Informational (D5). |
| `float` | number | IEEE-754. The mod's source values are C# `float` (32-bit), so a Go `float64` round trip is lossless in one direction only. No side compares two floats for exact equality — where this contract says two floats are equal, it means the relative test of §13, A10. `NaN` and `±Inf` are **forbidden** anywhere in this contract; a frame carrying one is malformed. |
| `simTick` | number | Int64. The source sim's tick counter. Informational, and not comparable across sims (D5). |

### 4.2 The edge enum

`"N"`, `"S"`, `"E"`, `"W"`. Uppercase, single character. Any other value fails validation.

The opposite-edge function, which the **sidecar** owns, is
`N↔S`, `E↔W`.

Under the ring (D8) a sim uses exactly two of the four values: `"E"` is the **export edge**
and `"W"` is the **passive entry edge** (amended — §14, A11). All four values stay in the
enum — removing one would be a major bump, the retired `{x, y}` grid needs them if it ever
returns, and M6's payload kinds may not share the ring's geometry.

### 4.3 Border position — the exact formula

`exitPosition` and `entryPosition` are a normalized coordinate **along** an edge, in
`[0.0, 1.0]`. `S` is the sim's half-extent,
`ScenarioIndependentSettings.Instance.SimulationSize.val` (`m2_findings.md` §2.1). The
playable square is `[−S, +S] × [−S, +S]`.

The free coordinate of an edge, and the mapping, are:

| Edge | Fixed coordinate | Free coordinate | Normalized position |
|---|---|---|---|
| `N` | `y = +S` | `x` | `(x + S) / (2S)` |
| `S` | `y = −S` | `x` | `(x + S) / (2S)` |
| `E` | `x = +S` | `y` | `(y + S) / (2S)` |
| `W` | `x = −S` | `y` | `(y + S) / (2S)` |

The inverse, which the receiving mod applies with **its own** `S`:

| Edge | Free world coordinate |
|---|---|
| `N`, `S` | `x = 2S · position − S` |
| `E`, `W` | `y = 2S · position − S` |

Rules:

- The sender **MUST** clamp to `[0.0, 1.0]` before sending. An organism a little past the
  corner produces a value slightly outside the range; clamp, do not reject.
- A receiver **MUST** reject a value outside `[0.0, 1.0]` as invalid, because a valid
  sender always clamps.
- The **sidecar** never converts a normalized position into a world coordinate. It has no
  business knowing `S`, the strip width `W`, or the inset margin. It copies the number.
- The **mod** computes the absolute entry point. For an entry on edge `W` it uses
  `x = −S + W + margin`, and the free coordinate from the table above
  (`m2_findings.md` §4.4, §(c)).

> **Note — a refinement of the one-line description in `system_decomposition.md`.**
> Contract C describes `MIGRATE_IN` as carrying "entry coords". This specification sends
> a **normalized** `entryEdge` + `entryPosition` instead of absolute coordinates, because
> only the mod knows its own `S`, its strip width, and its inset margin. Absolute
> coordinates chosen by the sidecar would land the organism inside the receiving sim's
> own border strip and it would migrate straight back (`m2_findings.md` §4.4).

### 4.3.1 The capture band — where an export may be taken (added — §14, A13)

M2 captured an organism **inside** the square, in the strip `S − W ≤ x ≤ S` on edge `E`. M3
keeps `worldWrapping` ON (D10), and the wrap teleports at `r − bodyLength ≥ 1.5·S + 1000`.
A fast organism can clear the whole strip in one `FixedUpdate` and land outside the square,
where M2's rule no longer captures it — and the wrap then teleports it to the antipode,
which a player reads as a defect in the mod (`m3_considerations.md`, Risk 2).

The capture band on the export edge `E` is therefore **half-open and outward-unbounded**:

```
capture(organism)  ⇔  x ≥ S − W                    the band starts at the strip line
                   ∧  velocity.x > 0               and it must be leaving
                   ∧  not already in flight
                   ∧  entry immunity has expired
```

| Rule | Value |
|---|---|
| Inner boundary | `x = S − W`, with `W` = `borderWidth` |
| Outer boundary | **none.** The band runs from the strip line outward, past `x = S`, past the wrap radius, to any `x` |
| Direction test | `velocity.x > 0` on edge `E`. **REQUIRED everywhere in the band, not only outside the square** |
| Test cadence | Every `FixedUpdate`, on every live organism. **Never** on a slower timer |

Three consequences, all load-bearing:

- **The outward-velocity test is what separates an export from a wrap.** A wrapped organism
  arrives in the outer band on the far side and travels **inward**, so it fails the test.
  Remove the test and every wrapped arrival immediately exports through the edge it landed
  behind (`m3_considerations.md`, Risk 1).
- **The cadence is what makes the band beat the wrap.** The wrap fires inside
  `BibitePropulsion.UpdateOrgan`, which runs in the organism's own tick. A capture test on a
  coroutine or a once-per-second timer loses the race for a fast organism.
- **`exitPosition` needs the clamp of §4.3, and now it really fires.** Inside the square a
  diagonal escape produces a value a little outside `[0, 1]`; at `x = 3500, y = 6000` with
  `S = 2000` it produces `2.0`. The sender **MUST** clamp to `[0.0, 1.0]`; the receiver
  **MUST** still reject an unclamped value, because a valid sender always clamps. Neither
  rule changed — the band is what makes them matter.

The entry edge `W` has **no capture band at all**. It is passive: an organism that walks
west out of the square is not a migrant, and the wrap returns it (D10, §14 A11).

> **OPEN — the direction test has no magnitude floor** (§12 item 7). `velocity.x > 0` is a
> strict comparison against zero and nothing more, so an organism that is loitering in the
> band and merely jittering eastward on one `FixedUpdate` exports. That is **not** a defect
> and this specification does **not** add a floor in M3: any floor is a second tunable, it
> has to be expressed in `S` to survive a different `SimulationSize`, and a floor set wrong
> turns a legitimate slow crossing into an organism pinned against the edge forever. The
> one-way ring also makes the failure mode cheap — a jitter export moves the organism
> forward, never back, so it cannot ping-pong (Risk 3). Revisit only if it turns out to be
> operationally noisy: the evidence would be a band population that churns across the ring
> without any of them travelling anywhere, and the fix would be a floor on the **outward
> component over several ticks**, not on one sample.

### 4.4 Velocity and heading

| Field | Frame of reference |
|---|---|
| `velocity` | `{"x": float, "y": float}`, world units per **simulated** second, in the source sim's world axes. Read from `Rigidbody2D.linearVelocity`. |
| `heading` | float, **degrees**, counter-clockwise, `0` = the `+y` axis. This is `transform.localRotation.eulerAngles.z` and `rb2d.r`, which the game keeps equal. The organism's forward direction is `transform.up` (`m2_findings.md` §1.1). |

**Velocity and heading are copied, never mirrored.** An organism that leaves eastward
enters the next slot still travelling eastward. That continuity is the whole point of
D3's map-edge model. The sidecar **MUST NOT** negate, rotate, or reflect either value in
M2, and **MUST NOT** in M3 either, because every ring slot is a pure translation of every
other (amended — §14, A11).

**`heading` is NOT range-normalized, and no implementation may assume `[0, 360)`.** The
value is whatever `Rigidbody2D.rotation` holds, and Unity lets that accumulate without
bound: real M3 emissions included `heading = 2768.18`. Two rules follow, and both sides
already obey them:

- A **sender** copies the raw value. It **MUST NOT** wrap, fold or normalize it. Wrapping
  would be a silent behaviour change on a field whose only consumer treats it as an angle.
- A **receiver** feeds it straight to rotation — `Quaternion.Euler(0, 0, heading)` and
  `Rigidbody2D.rotation`, both of which take any real number and reduce it themselves. It
  **MUST NOT** validate a range, and a value outside `[0, 360)` is **never** a NACK reason.

The only test either side may apply is finiteness (§4.1): `NaN` and `±Inf` are forbidden on
the wire, and every other float is a legal heading. `exitPosition` is the field with a
range; `heading` is not, and the two must not be validated alike.

### 4.5 The `kind` enum

`"bibite"` is the only value M2 and M3 accept. `"corpse"`, `"pellet"` and `"egg"` are
reserved for **M6** (Contract C, §6.4 of the research) — the milestone renumbering of D9
moved them one place out. A receiver that reads a reserved-but-
unsupported kind answers with the NACK code `KIND_UNSUPPORTED`. It does not close.

### 4.6 The payload string

`payload` is the game's own Newtonsoft output for one organism, carried as a **JSON
string** (D4). It is produced by `SaveSystem.SerializeBibite` plus a `version` key, and
consumed by `SaveSystem.LoadBibiteOrEggFromData` (`m1_findings.md` §1, §2).

- It **MUST** be sent as a string, not as a nested object. Nesting it would force the mod
  to have an opinion about its shape, which D4 forbids.
- It **MUST NOT** be base64-encoded. It is already text. Standard JSON string escaping is
  the only transformation.
- Maximum length is `maxPayloadBytes` (4 MiB) measured in UTF-8 bytes. An oversize
  payload is answered with `INVALID_PAYLOAD`, not with a close.
- `gameVersion` travels beside it in the same message. The sidecar uses the pair to pick a
  `bb8-schema` dialect. It **MUST NOT** trust the `version` key inside the blob over the
  `gameVersion` field beside it; the field is authoritative.

In the examples below, `{ ... }` inside a payload string marks elided content. It is not
literal.

---

## 5. Message catalogue

Nine types. Five from mod to sidecar, four from sidecar to mod.

| Type | Direction | Answered by |
|---|---|---|
| `CONFIG_UPDATE` | mod → sidecar | nothing (but triggers `EDGE_STATUS`) |
| `HEARTBEAT` | mod → sidecar | nothing |
| `MIGRATE_OUT` | mod → sidecar | `MIGRATE_OUT_ACK` or `MIGRATE_OUT_NACK` |
| `MIGRATE_IN_ACK` | mod → sidecar | nothing |
| `MIGRATE_IN_NACK` | mod → sidecar | nothing |
| `EDGE_STATUS` | sidecar → mod | nothing |
| `MIGRATE_IN` | sidecar → mod | `MIGRATE_IN_ACK` or `MIGRATE_IN_NACK` |
| `MIGRATE_OUT_ACK` | sidecar → mod | nothing |
| `MIGRATE_OUT_NACK` | sidecar → mod | nothing |

---

### 5.1 `CONFIG_UPDATE` — mod → sidecar

**When sent.** As the **first frame on every connection** — it is the handshake — and
again whenever any field in it changes. It is not periodic.

The mod **MUST** send `CONFIG_UPDATE` before any other frame. The sidecar **MUST** treat
any other first frame as malformed and close with `4003`.

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `sessionId` | `uuid` | yes | Minted fresh on **every world load**. A change in `sessionId` tells the sidecar the mod lost all in-memory state and that the world may have rolled back to an earlier save. This drives custody reassertion (§7.4). The game's own `gameName` is unreliable for this — `CreateSave` never writes one (`m1_findings.md`, hazards) — so the mod mints its own value. |
| `reason` | string enum | yes | `"connect"`, `"world_loaded"`, `"settings_changed"`, `"sim_size_changed"`. Informational for the sidecar's logs, except that `"connect"` marks the handshake frame. |
| `gameVersion` | string | yes | `UnityEngine.Application.version`, for example `"0.6.3.1"`. The sidecar rejects an unsupported value with close `4002`. |
| `modVersion` | string | yes | The plugin's own version, for example `"0.2.0"`. Informational. |
| `simulationSize` | float | yes | `S`, the playable half-extent. Read live from the setting, never cached (`m2_findings.md` §2.4). |
| `borderEdges` | array of edge enum | yes | The edges on which the mod has a border strip and can therefore **accept an inbound organism**. Under the ring it is exactly `["E", "W"]`: the export edge and the passive entry edge (amended — §14, A11). It no longer means "can migrate out" — `exportEdge` says that. The sidecar **MUST NOT** deliver a `MIGRATE_IN` on an edge absent from this list (§9.2, `EDGE_CLOSED`, §13 A3). |
| `exportEdge` | edge enum | yes, from `1.1` | **The one edge this sim exports through** (added — §14, A11). `"E"` under the ring (D8). It **MUST** be a member of `borderEdges`; a frame where it is not is unusable and closes with `4003` (§13, A8). The sidecar **MUST NOT** open any other edge, whatever `borderEdges` says. **Absent** — a `contract-a/1` mod — is not a close: the sidecar takes the single member of `borderEdges` as the export edge, and closes `4003` only when `borderEdges` does not have exactly one entry. |
| `borderWidth` | float | yes | `W`, the strip width in world units, and the inner boundary of the capture band (§4.3.1). Informational for the sidecar; the mod owns the geometry. |
| `ringSlot` | number (int) | no | The mod's configured ring slot, from its environment. Advisory, `≥ 1`. When present and it disagrees with the slot the sidecar holds, the sidecar **MUST** close with `4001`. This catches a mis-wired three-instance rig in one second instead of one hour (added — §14, A14). |
| ~~`sector`~~ | object `{x:int, y:int}` | no | **Retired** (amended — §14, A14). D8 retires the `{x, y}` grid. A `contract-a/1.1` mod **MUST NOT** send it; a `contract-a/1.1` sidecar **MUST** ignore it if an older mod does, and **MUST NOT** close on it. Replaced by `ringSlot`. |
| `worldName` | string | no | Cosmetic. Often empty for a world the game itself saved. Never used as an identifier. |

**Receiver obligations.** On the handshake frame the sidecar validates `gameVersion`, then
`ringSlot`, then `exportEdge ∈ borderEdges`, then sends exactly one `EDGE_STATUS` (§5.4).
On a later `CONFIG_UPDATE` that changes `simulationSize`, `borderEdges` or `exportEdge`,
the sidecar re-validates peer agreement and sends a new `EDGE_STATUS` whenever the result
changed (amended — §14, A11, A14).

```json
{
  "protocol": "contract-a/1.1",
  "type": "CONFIG_UPDATE",
  "messageId": "1c2fbe80-5a17-4a2b-9a20-3d54f1b7e001",
  "sentAt": 1785693598004,
  "data": {
    "sessionId": "9a4c1e77-0b3d-4f52-8c19-6d2e7f0a5b31",
    "reason": "connect",
    "gameVersion": "0.6.3.1",
    "modVersion": "0.2.0",
    "simulationSize": 2000.0,
    "borderEdges": ["E", "W"],
    "exportEdge": "E",
    "borderWidth": 60.0,
    "ringSlot": 1,
    "worldName": "M3-Slot1"
  }
}
```

---

### 5.2 `HEARTBEAT` — mod → sidecar

**When sent.** Every `heartbeatIntervalMs` (default 1000 ms) of **wall-clock** time, on a
timer that does not depend on the simulation. A paused sim still heartbeats. See §8.

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `sessionId` | `uuid` | yes | Same value as the current `CONFIG_UPDATE`. A mismatch means the mod skipped a handshake; the sidecar closes with `4003`. |
| `simTick` | number (int64) | yes | The sim's current tick. Informational, and never comparable across sims (D5). |
| `simulatedTime` | float | yes | `TimeKeeper.simulatedTime`, in simulated seconds. Informational. |
| `population` | number (int) | yes | Live organism count — `BibiteTracker.instance.bibites` filtered for non-null. Feeds the sidecar's admission control. |
| `eggCount` | number (int) | no | Live egg count. Feeds admission control when present. |
| `paused` | bool | yes | `TimeController.paused`. While `true` the sidecar **MUST NOT** count a missing `MIGRATE_IN_ACK` against the mod. |
| `timeScale` | float | yes | The effective time scale. `0` means stopped. |
| `simulationSize` | float | yes | `S`, read live. A change here **MUST** make the sidecar re-check peer agreement, exactly as a `CONFIG_UPDATE` would. This is the belt to `CONFIG_UPDATE`'s braces, because `SimulationSize` is live-mutable (`m2_findings.md` §2.4). |
| `inFlightOut` | number (int) | no | How many `MIGRATE_OUT`s the mod is currently waiting on. Useful for diagnosing a stuck custody chain. |
| `pendingIn` | number (int) | no | How many `MIGRATE_IN`s are queued in the mod but not yet spawned. |

**Receiver obligations.** The sidecar records the arrival time and the counters. It sends
nothing back.

```json
{
  "protocol": "contract-a/1.1",
  "type": "HEARTBEAT",
  "messageId": "6b0a3f1d-3c4e-4a91-b7f2-51c8a0d33e42",
  "sentAt": 1785693600000,
  "data": {
    "sessionId": "9a4c1e77-0b3d-4f52-8c19-6d2e7f0a5b31",
    "simTick": 4820311,
    "simulatedTime": 120507.75,
    "population": 214,
    "eggCount": 37,
    "paused": false,
    "timeScale": 1.0,
    "simulationSize": 2000.0,
    "inFlightOut": 0,
    "pendingIn": 0
  }
}
```

---

### 5.3 `MIGRATE_OUT` — mod → sidecar

**When sent.** An organism is inside the **capture band** of the mod's `exportEdge`, that
edge is **open**, the organism is moving outward, it is not already in flight, and its
entry immunity has expired. §4.3.1 states the band and the direction test in full; the band
now reaches outside the playable square (amended — §14, A13). The mod mints the
`migrationId` and binds it to that organism.

**Before sending, the mod MUST make the organism inert** and keep it inert until the
message resolves. See §6.3. The mod **MUST NOT** destroy it yet.

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | **The idempotency key (D2).** Minted by the mod, once per organism per migration attempt. It **MUST NOT** change across a retry of the same organism, including a retry after a reconnect. |
| `entityId` | `entityId` | yes | `BibiteBody.id.id`. Survives the round trip (`m1_findings.md` §4.1) and is the durable dedup key at the destination and the reconciliation key after a rollback (§7.4). |
| `kind` | string enum | yes | `"bibite"` in M2 and M3. |
| `gameVersion` | string | yes | The version that produced `payload`. Authoritative over the blob's own `version` key. |
| `payload` | string | yes | The opaque bb8 blob (§4.6). |
| `parents` | array of object | no | **The lineage inputs** (added — §14, A12). `0`–`maxParentBlobs` (2) entries, in `BibiteGenes.parent1` then `BibiteGenes.parent2` order. Absent and `[]` mean the same thing: no parent is known. |
| `parents[].entityId` | `entityId` | yes | The parent's `BibiteBody.id.id`, read from the **live `BibiteGenes` component** of the migrant — never from the migrant's serialized payload (amended — §14, A12). `0` is the game's "unassigned" sentinel and such an entry **MUST** be omitted entirely. |
| `parents[].payload` | string | no | The parent's own opaque bb8 blob, from `SaveSystem.SerializeBibite`, subject to the same rules as `payload` (§4.6). **Absent means the parent is gone** — `BibiteGenes` drops the parentage once the parent GameObject is destroyed, so this is normal and is recorded as a gap, never as an error. |
| `parents[].gameVersion` | string | no | The version that produced `parents[].payload`. Absent means "the same as the migrant's `gameVersion`", which is always true in practice because both were serialized in the same tick. |
| `exitEdge` | edge enum | yes | Which of the mod's own edges the organism crossed. Always the mod's `exportEdge` — `"E"` under the ring. |
| `exitPosition` | float | yes | `[0,1]` along that edge, by the formula in §4.3. |
| `velocity` | object `{x,y}` | yes | World velocity at the moment of capture (§4.4). |
| `heading` | float | yes | Degrees (§4.4). |
| `simulationSize` | float | yes | `S` at the moment of capture. The sidecar uses it to refuse a transfer to a peer with a different `S`. |
| `simTick` | number (int64) | yes | Informational. |

**Receiver obligations, in this order.** The sidecar **MUST**:

1. Validate the frame's fields, then validate `payload` with `bb8-schema` against
   `gameVersion`. On failure reply `MIGRATE_OUT_NACK` / `INVALID_PAYLOAD`.
2. Check `migrationId` against the journal. If an entry already exists **with the same
   payload hash**, reply `MIGRATE_OUT_ACK` immediately and journal nothing. This is what
   makes a retry after a lost ACK safe.
3. If an entry exists with a **different** payload hash, reply `MIGRATE_OUT_NACK` /
   `DUPLICATE_MIGRATION_ID`. That is a mod defect and it must be loud.
4. Check that `exitEdge` is currently open and that the neighbour's `S` equals
   `simulationSize`, by the relative test of §13, A10 — never by `==`
   (amended — §13, A10). On failure reply the matching NACK from §9.1.
5. Build the **lineage annex** from `payload` and `parents` (added — §14, A12): hash the
   migrant's genome and every present `parents[].payload` with `bb8-schema`'s canonical
   projection (`genome-hash.md`), cache each blob in the genome cache under its hash, and
   record a **gap** for every parent entry that carries no blob or whose blob will not
   hash. This step **MUST NOT** fail the migration: a gap is a normal outcome (D11).
6. Write the journal entry and **flush it to durable storage** (`fsync` the file and its
   directory). Only then reply `MIGRATE_OUT_ACK`. An ACK before the flush breaks D2.
7. Forward over Contract B, with the annex attached and **every parent blob stripped**
   (`contract-b-m3.md` §6.6). Never before step 6.

**Sender obligations for `parents`** (added — §14, A12). The mod **MUST**:

- read the parent entity IDs from the migrant's **live `BibiteGenes` component**, in
  `parent1` then `parent2` order: the `GameObject` reference resolved to
  `BibiteBody.id.id` while it is alive, and the component's own `parent1ID` / `parent2ID`
  when it is not. **Not** from the migrant's serialized payload — the game omits
  `$.genes.parent1` / `$.genes.parent2` entirely when the parent `GameObject` is gone
  (`BibiteGenes.SaveState`, `BibiteGenes.cs:552-559`), so a payload-sourced reader loses
  exactly the entries that matter (amended — §14, A12);
- serialize a parent with `SaveSystem.SerializeBibite` **only while its GameObject is
  alive**, in the same `FixedUpdate` as the migrant's own serialization;
- **never parse, inspect or hash** a parent blob. D4 is unchanged: the blob is opaque to
  the mod, and the hash is the sidecar's job;
- keep the whole frame under `maxFrameBytes`. When the migrant plus its parents would
  exceed `maxFrameBytes − frameHeadroomBytes`, the mod **MUST** drop parent **blobs**,
  largest first, keeping their `entityId` entries. A dropped blob is a gap, and a gap is
  never a reason to abandon a migration. The migrant's own `payload` is never dropped;
- treat a serialization failure for a parent as a gap, not as an error.

An implementation **SHOULD** cache the serialized blob by entity ID **within one tick**: a
brood of siblings crossing together would otherwise serialize the same mother repeatedly on
the main thread (`m3_considerations.md`, Risk 8).

```json
{
  "protocol": "contract-a/1.1",
  "type": "MIGRATE_OUT",
  "messageId": "d3a11c9e-77b4-4b2f-8e5c-0a91f4d6b210",
  "sentAt": 1785693600123,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "entityId": -843827577,
    "kind": "bibite",
    "gameVersion": "0.6.3.1",
    "payload": "{\"transform\":{\"position\":[2412.6,6003.1],\"rotation\":274.11,\"scale\":0.9312},\"rb2d\":{\"px\":2412.6,\"py\":6003.1,\"vx\":61.2,\"vy\":4.4,\"r\":274.11},\"genes\":{\"parent1\":-1180911975, ... },\"body\":{\"id\":-843827577, ... },\"clock\":{ ... },\"brain\":{ ... },\"version\":\"0.6.3.1\"}",
    "parents": [
      {
        "entityId": -1180911975,
        "payload": "{\"transform\":{ ... },\"rb2d\":{ ... },\"genes\":{ ... },\"body\":{\"id\":-1180911975, ... },\"clock\":{ ... },\"brain\":{ ... },\"version\":\"0.6.3.1\"}",
        "gameVersion": "0.6.3.1"
      },
      { "entityId": 204418833 }
    ],
    "exitEdge": "E",
    "exitPosition": 1.0,
    "velocity": { "x": 61.2, "y": 4.4 },
    "heading": 274.11,
    "simulationSize": 2000.0,
    "simTick": 4820344
  }
}
```

The example is a capture from **outside** the square (§4.3.1): the organism sits at
`x = 2412.6, y = 6003.1` with `S = 2000`, having cleared the strip in one tick at
`vx = 61.2`. The raw normalized position would be `(6003.1 + 2000) / 4000 = 2.0008`, and
the mod clamps it to `1.0` before sending. The second parent is a **gap**: its `GameObject`
is gone, so no blob accompanies it — and note that the migrant's own `payload` no longer
carries a `parent2` key either, because `BibiteGenes.SaveState` writes one only for a live
parent. The entity ID `204418833` comes from the component's `parent2ID`, which is why the
mod reads the component and not the blob.

**A gap entry is rarer than that example makes it look.** `parent1ID` / `parent2ID` are
`[NonSerialized]` and are only ever filled by `BibiteGenes.LoadState`, from a payload that
still carried the key. So an organism **born in this world** whose parent later died
contributes **no entry at all** — the `GameObject` is fake-null and the recorded integer is
`0`, which §5.3 omits. A gap entry needs an organism that was **restored** — from a save,
or from an earlier hop of this ring — while its parent was still alive at that serialization.
Every hop after the first therefore produces gaps for parents that hop 1 shipped as blobs,
which is exactly what the archive's genome store is for.

---

### 5.4 `EDGE_STATUS` — sidecar → mod

**When sent.** Once immediately after the handshake `CONFIG_UPDATE`, and again on every
change: the east neighbour connecting or dying, a ring insertion or release, an `S`
disagreement, or an operator closing the edge (amended — §14, A11).

`EDGE_STATUS` is **full state, not a delta**. Under the ring it carries **exactly one
entry — the mod's `exportEdge`** (amended — §14, A11). The entry edge is passive: it
always accepts an inbound organism and never appears here.

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `epoch` | number (int64) | yes | Strictly increasing per connection, starting at 1. The mod **MUST** ignore an `EDGE_STATUS` whose `epoch` is lower than or equal to the last one it applied. This makes the message order-independent and replay-safe. The counter resets on a new connection. |
| `edges` | array of object | yes | **Exactly one entry**, for the mod's `exportEdge` (amended — §14, A11). An empty array closes the export edge and is the correct frame to send when the sidecar holds no ring slot. |
| `edges[].edge` | edge enum | yes | Which edge. **MUST** equal the `exportEdge` the mod declared. |
| `edges[].open` | bool | yes | `true` means migration out of this edge is permitted right now. |
| `edges[].reason` | string enum | yes | Why. When `open` is `true`: `"peer_live"`. When `open` is `false`: `"no_peer"`, `"peer_mod_absent"` (added — §14, A11), `"peer_incompatible"`, `"peer_unreachable"`, `"peer_overloaded"`, `"admin_closed"`, `"sim_size_mismatch"`. |
| `edges[].peerSimulationSize` | float | no | Present when `open` is `true`. The **east neighbour's** `S`. The mod **MUST** compare it against its own `S` and treat the edge as closed on a mismatch, even though the sidecar already checked. Two independent checks, because a mid-run resize can race. |

`contract-b-m3.md` §8 gives the exact mapping from the relay's `PEER_STATUS` to these
`open`/`reason` values. It is the sidecar's decision alone; the mod never sees a peer.

**Receiver obligations.** Until the mod has applied its first `EDGE_STATUS`, **the export
edge is closed** and the mod **MUST NOT** send `MIGRATE_OUT`. The same holds while
disconnected. This is the fail-safe: a mod that cannot reach its sidecar quietly stops
migrating instead of losing organisms.

An `edges` array with more than one entry is a sidecar defect. The mod **MUST** apply the
entry whose `edge` equals its own `exportEdge`, **MUST** ignore every other entry, and
**MUST** log one warning. It never closes the connection over it: an extra entry is a
forward-compatible shape under a later ring extension, not a fault.

The mod also uses the open/closed state to drive its edge behaviour at that edge —
void-avoidance scoping (`m2_findings.md` §1.5). It **no longer** drives a `worldWrapping`
override: under D10 the wrap stays ON at all times and the mod only snapshots and reports
the setting (amended — §14, A13). The `m2_findings.md` §3 advice to disable the wrap while
an edge is open is superseded.

```json
{
  "protocol": "contract-a/1.1",
  "type": "EDGE_STATUS",
  "messageId": "5e18b2c0-4a6d-4f88-9c31-b0e75a2d4413",
  "sentAt": 1785693598041,
  "data": {
    "epoch": 1,
    "edges": [
      {
        "edge": "E",
        "open": true,
        "reason": "peer_live",
        "peerSimulationSize": 2000.0
      }
    ]
  }
}
```

---

### 5.5 `MIGRATE_OUT_ACK` — sidecar → mod

**Meaning: the organism is durably journaled. The sidecar holds custody. Destroy it
now.** This is the single point at which custody transfers (D2).

**When sent.** Three cases:

1. In answer to a `MIGRATE_OUT` that passed validation and was flushed to the journal.
2. In answer to a `MIGRATE_OUT` whose `migrationId` was already journaled with the same
   payload hash — an idempotent repeat.
3. **Unsolicited**, as a custody reassertion after a mod session rollback (§7.4).

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The key from the `MIGRATE_OUT`. |
| `entityId` | `entityId` | yes | The organism's ID. **REQUIRED even though the mod usually knows it**, because case 3 arrives with no matching in-flight record and `entityId` is the only handle the mod has left. |
| `journaledAt` | `timestampMs` | yes | When the journal write was flushed. Informational. |
| `unsolicited` | bool | no | Default `false`. `true` marks case 3. The mod uses it only for logging; the required action is identical. |

**Receiver obligations.** The mod **MUST**:

1. Look up its in-flight record by `migrationId`. If found, destroy that organism.
2. If not found, scan the live world for an organism whose `BibiteBody.id.id` equals
   `entityId`. If found, destroy it — the sidecar holds custody of it and the local copy
   is a rollback artefact.
3. If neither is found, log one line and do nothing. Custody already moved.

The destruction **MUST** be a raw `UnityEngine.Object.Destroy(body.gameObject)` after
removing the body from `BibiteTracker.instance.bibites` — no corpse, no meat, no eggs,
and clean death statistics (`m1_findings.md` §5). It **MUST** run on the Unity main
thread inside `FixedUpdate`, never on the socket thread.

There is no acknowledgement of an acknowledgement. The chain stops here.

```json
{
  "protocol": "contract-a/1.1",
  "type": "MIGRATE_OUT_ACK",
  "messageId": "a0c47f21-6b19-4d05-93ae-1c8f2b6e5507",
  "sentAt": 1785693600141,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "entityId": -843827577,
    "journaledAt": 1785693600139,
    "unsolicited": false
  }
}
```

---

### 5.6 `MIGRATE_OUT_NACK` — sidecar → mod

**Meaning: the sidecar did not take custody. The organism is still the mod's.**

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The key from the `MIGRATE_OUT`. |
| `entityId` | `entityId` | yes | Echoed for logging and for the mod's own lookup. |
| `code` | string enum | yes | One of §9.1. |
| `class` | string enum | yes | `"transient"` or `"permanent"`. Redundant with `code` on purpose: it lets a mod handle a code it does not recognise. An unrecognised code is treated as its stated `class`. |
| `message` | string | yes | Human-readable, for the BepInEx log. Never parsed. |
| `retryAfterMs` | number (int) | no | Present on transient codes. The mod **MUST NOT** retry this organism before this delay elapses. |

**Receiver obligations.** The mod **MUST** revive the organism (§6.3), clear its in-flight
record, and apply the cooldown. On a `"permanent"` class it **MUST** additionally mark
that organism as non-migratable for the rest of the session, so it does not spin against
the strip forever.

```json
{
  "protocol": "contract-a/1.1",
  "type": "MIGRATE_OUT_NACK",
  "messageId": "cf51d7a3-882b-4e14-a0d6-33b9c4e17708",
  "sentAt": 1785693600138,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "entityId": -843827577,
    "code": "EDGE_CLOSED",
    "class": "transient",
    "message": "edge E has no live neighbour: ring slot 2 last seen 42s ago",
    "retryAfterMs": 15000
  }
}
```

---

### 5.7 `MIGRATE_IN` — sidecar → mod

**When sent.** The sidecar holds a journaled inbound organism and the mod is connected and
handshaken. The sidecar **MUST** send the handshake's `EDGE_STATUS` before any
`MIGRATE_IN` on that connection (§6.1); the mod does **not** gate an inbound delivery on
having applied one, because a bounce-back can be the very first frame it sees.

Also sent for a **bounce-back**: an organism the local sim exported that the sidecar has
proved is not at its destination (custody chain step 6). "Proved" is narrow, and §13, A6
defines it — a delivery timeout on its own is **not** proof and **MUST NOT** bounce
(amended — §13, A6).

The sidecar **MUST** send inbound organisms in journal order and **MUST NOT** have more
than `inboundQueueMax` un-ACKed deliveries outstanding.

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The idempotency key. Preserved end to end from the originating mod. |
| `entityId` | `entityId` | yes | Extracted from the blob by `bb8-schema`. The mod uses it as its **durable** dedup key (§7.3) and never parses the blob itself (D4). |
| `kind` | string enum | yes | `"bibite"` in M2 and M3. Unknown kind → `MIGRATE_IN_NACK` / `KIND_UNSUPPORTED`. |
| `gameVersion` | string | yes | The version the blob is valid for, after any sidecar-side conversion. The mod compares it against `Application.version`. |
| `payload` | string | yes | The opaque bb8 blob (§4.6), already validated by `bb8-schema`. |
| `entryEdge` | edge enum | yes | The edge of the **receiving** sim the organism enters through. The sidecar computes it; the mod does not derive it. Under the ring it is `"W"` for ordinary traffic — the passive entry edge — and the sim's **own `exportEdge`** for a bounce-back, because a bounce-back comes home through the door it left by (amended — §14, A11; `contract-b-m3.md` §9). |
| `entryPosition` | float | yes | `[0,1]` along `entryEdge`, by §4.3. In M2 the sidecar copies `exitPosition` unchanged. |
| `velocity` | object `{x,y}` | yes | Copied, never mirrored (§4.4). |
| `heading` | float | yes | Copied, never mirrored (§4.4). |
| `bounceBack` | bool | yes | `true` when this organism is coming home after a failed remote delivery. The spawn behaviour is identical; the mod logs it and still applies the entry-immunity window. |
| `attempt` | number (int) | yes | Delivery attempt, starting at 1. Incremented on each replay. Log-only. Dedup is on `migrationId` and `entityId`, never on `attempt`. |
| `ackDeadlineMs` | number (int) | no | How long the sidecar waits before it re-queues this delivery. Default `migrateInAckTimeoutMs`. Advisory. |

**Receiver obligations.** The mod **MUST** enqueue the message on a thread-safe queue and
process it on the Unity main thread inside `FixedUpdate`. `FixedUpdate` does not run at
`timeScale` 0, so a delivery that has waited more than two wall-clock seconds in that
queue while the sim is stopped is answered `MIGRATE_IN_NACK` / `SIM_NOT_READY` from
`Update` instead of waiting for a tick that will not come (amended — §13, A4).

A `MIGRATE_IN` whose `migrationId` is absent, not a string, or empty **MUST** be dropped
with one logged error and no reply — every reply is keyed on that field, so the frame has
no answer channel (amended — §13, A2).

Processing order:

1. Deduplicate (§7.3). On a hit, reply `MIGRATE_IN_ACK` with `duplicate: true` and spawn
   nothing.
2. Compute the world entry point from `entryEdge`, `entryPosition`, its own `S`, its own
   strip width and inset margin (§4.3, `m2_findings.md` §(c)).
3. Rewrite the eight position numbers in the payload JSON — `$.transform.position[0]`,
   `$.transform.position[1]`, `$.transform.rotation`, `$.rb2d.px`, `$.rb2d.py`,
   `$.rb2d.vx`, `$.rb2d.vy`, `$.rb2d.r` — then call
   `SaveSystem.instance.LoadBibiteOrEggFromData(json, true, null, null)`.
4. **In the same frame**, re-assert `transform.position`, `transform.rotation`,
   `rb.position`, `rb.linearVelocity` and `rb.rotation` directly. The `Rigidbody2D` wins
   over the transform on the next tick, and the parent transform of `bibiteHolder` is
   still unproven (`m2_findings.md` §4.3).
5. Repair `genes.parent1` / `parent2` and any parent's `eggLayer.children`, mirroring
   `SaveSystem.cs:748-782` (`m1_findings.md` §4.3).
6. Start the entry-immunity window keyed on `entityId`.
7. Reply `MIGRATE_IN_ACK`, or `MIGRATE_IN_NACK` on any failure.

Step 6 is **REQUIRED under the ring, not optional** (amended — §14, A11). The one-way lanes
already make an immediate re-export geometrically impossible for ordinary traffic — an
arrival lands in the west entry strip and the capture band is on the east side — but a
**bounce-back** lands inside the capture band it left from, moving outward. Without the
immunity window it re-exports on the next tick.

A `null` return from `LoadBibiteOrEggFromData` is the normal failure signal — the method
swallows every exception (`m1_findings.md` §1.2). Reply `DESERIALIZE_FAILED`.

```json
{
  "protocol": "contract-a/1.1",
  "type": "MIGRATE_IN",
  "messageId": "7f2b91d6-0e34-4c7a-b158-9a03e6c2f411",
  "sentAt": 1785693600187,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "entityId": -843827577,
    "kind": "bibite",
    "gameVersion": "0.6.3.1",
    "payload": "{\"transform\":{\"position\":[2000.0,412.77],\"rotation\":274.11,\"scale\":0.9312},\"rb2d\":{\"px\":2000.0,\"py\":412.77,\"vx\":6.12,\"vy\":0.44,\"r\":274.11},\"genes\":{ ... },\"body\":{\"id\":-843827577, ... },\"clock\":{ ... },\"brain\":{ ... },\"version\":\"0.6.3.1\"}",
    "entryEdge": "W",
    "entryPosition": 0.6031925,
    "velocity": { "x": 6.12, "y": 0.44 },
    "heading": 274.11,
    "bounceBack": false,
    "attempt": 1,
    "ackDeadlineMs": 10000
  }
}
```

---

### 5.8 `MIGRATE_IN_ACK` — mod → sidecar

**Meaning: the organism is alive in this world, or it was already here. The sidecar can
release its custody.**

The name in `system_decomposition.md` says "spawned and overwritten". There is no
overwrite step: `LoadBibiteOrEggFromData` restores full live state and preserves the
entity ID with no mutation (`m1_findings.md` §1.2). The ACK means **restored and
re-linked**.

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The key from the `MIGRATE_IN`. |
| `entityId` | `entityId` | yes | The **restored** organism's `BibiteBody.id.id`. A value that differs from the `MIGRATE_IN`'s `entityId` is a defect, and the sidecar logs it loudly. |
| `duplicate` | bool | yes | `true` when the mod deduplicated and spawned nothing. The sidecar treats it exactly like a normal ACK — it clears the journal entry and sends `MIGRATION_ACK` upstream. |
| `simTick` | number (int64) | yes | The tick the organism entered on. Informational. |
| `relinkedParents` | number (int) | no | How many parent references were repaired. |
| `relinkedChildren` | number (int) | no | How many child references were repaired. Both counters exist because the M1 exit test never exercised this code path, and M2 must prove it ran. |

**Receiver obligations.** The sidecar deletes its journal entry for `migrationId` and
sends `MIGRATION_ACK` over Contract B, which lets the origin peer clear its own journal
(custody chain step 5). It **MUST** retain a tombstone for `exportRetentionSeconds`, so a
later replay of the same `migrationId` is answered without a second delivery.

```json
{
  "protocol": "contract-a/1.1",
  "type": "MIGRATE_IN_ACK",
  "messageId": "34ab7c05-1d92-4e60-8b47-c1f0d5a29316",
  "sentAt": 1785693600231,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "entityId": -843827577,
    "duplicate": false,
    "simTick": 3910772,
    "relinkedParents": 2,
    "relinkedChildren": 3
  }
}
```

---

### 5.9 `MIGRATE_IN_NACK` — mod → sidecar

**Meaning: the organism is not in this world. The sidecar keeps custody.**

The mod **MUST** guarantee that no partially-restored organism survives a NACK. If step 5
of §5.7 fails after step 3 succeeded, the mod destroys the half-restored organism before
replying.

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The key from the `MIGRATE_IN`. |
| `entityId` | `entityId` | yes | Echoed from the `MIGRATE_IN`. |
| `code` | string enum | yes | One of §9.2. |
| `class` | string enum | yes | `"transient"` or `"permanent"`. |
| `message` | string | yes | Human-readable. Never parsed. |
| `retryAfterMs` | number (int) | no | Present on transient codes. The sidecar **MUST NOT** re-deliver before this delay elapses. |

**Receiver obligations.** On a `"transient"` class the sidecar keeps the journal entry and
re-delivers after the delay, with `attempt` incremented. It **MUST** honour
`retryAfterMs`, and it **MUST NOT** count the silence of a paused mod as a missed
delivery (§8).

On a `"permanent"` class it **MUST NOT** re-deliver to this mod, and in M2 it
**MUST NOT** return the organism over Contract B either. It moves the journal entry to a
`held` state, keeps it there for an operator, and logs one loud line
(amended — §13, A7; `contract-b-m3.md` §9). It **MUST NOT** silently drop it — a drop is
the one failure mode D2 accepts, but it is never the first choice.

```json
{
  "protocol": "contract-a/1.1",
  "type": "MIGRATE_IN_NACK",
  "messageId": "e91d4f3b-7c60-4a25-91b8-40d7e2ca6b19",
  "sentAt": 1785693600244,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "entityId": -843827577,
    "code": "SIM_OVERLOADED",
    "class": "transient",
    "message": "population 2140 is above the admission ceiling of 2000",
    "retryAfterMs": 30000
  }
}
```

---

## 6. Connection lifecycle

### 6.1 Startup and handshake

```
mod                                   sidecar
 |  TCP + WebSocket upgrade  --------> |
 |  CONFIG_UPDATE(reason=connect) ---> |   validate gameVersion, ringSlot, exportEdge
 |  <-------------------- EDGE_STATUS  |   epoch = 1, full state
 |  HEARTBEAT ------------------------>|   (every 1000 ms from here on)
 |  <----------------------- MIGRATE_IN|   replay of un-ACKed journal entries
```

The mod does not migrate anything until it has applied an `EDGE_STATUS`.

### 6.2 Disconnect and reconnect

The mod is the reconnecting side. The sidecar never dials.

- Backoff: exponential with **full jitter**, from `reconnectBackoffMinMs` (1000 ms) to
  `reconnectBackoffMaxMs` (30000 ms). `delay = random(0, min(max, min·2^n))`.
- The backoff ladder `n` resets only after a session that **stayed up** for at least
  `stableSessionMs` (5000 ms). It **MUST NOT** reset on the TCP or WebSocket connect
  itself: a sidecar that accepts the socket and then closes it every time — close `4003`
  on an unusable `CONFIG_UPDATE` is the case that matters — would otherwise hold `n` at
  its floor and produce an unthrottled redial loop (amended — §13, A8).
- The connect attempt **MUST** run off the Unity main thread. A blocking connect on the
  main thread freezes the simulation.
- Close codes `4000`, `4001`, `4002` and `4006` stop reconnection until the mod is
  restarted or reconfigured. Every other close code reconnects with backoff.
- On every reconnect the mod sends a fresh `CONFIG_UPDATE`. The `sessionId` is unchanged
  when the world did not reload, and new when it did.
- The `EDGE_STATUS` `epoch` counter resets to 1 on a new connection. The mod resets its
  last-applied epoch when it opens a connection.

### 6.3 What the mod does with an organism in flight

The mod **MUST** make an organism inert from the moment it sends `MIGRATE_OUT` until the
message resolves. Inert means it cannot eat, breed, be eaten, move, or be seen as food.

- **Recommended implementation.** Remove the body from
  `BibiteTracker.instance.bibites`, set `bibiteBody.enabled = false` (which stops
  `BibiteBody.FixedUpdate`, and with it metabolism, brain ticks and organ updates —
  `m1_findings.md` §8), and set `rb2d.simulated = false` (which stops physics and
  colliders). Reverse all three to revive.
- This is what makes a temporary overlap harmless. If the ACK is lost and the connection
  drops, the organism exists locally **and** in the sidecar's journal until the reconnect
  resolves it — but the local copy does nothing during that window.
- On `MIGRATE_OUT_ACK`: destroy (§5.5).
- On `MIGRATE_OUT_NACK`: revive, apply the cooldown.
- On `migrateOutTimeoutMs` with no answer: **keep it inert, keep the `migrationId` bound
  to it**, and re-send the identical `MIGRATE_OUT` on the next connection. Do **not**
  mint a new `migrationId`, and do **not** revive it. Reviving it here is how you get a
  duplicate.
- On a connection close with an unresolved `MIGRATE_OUT`: same as the timeout. The
  organism stays inert until a sidecar answers. If the sidecar never comes back, the
  organism stays inert forever, which is loss — the acceptable failure under D2.

> One organism has at most one live `migrationId` at any time. The mod **MUST** enforce
> this. It is the mod-side half of at-most-once delivery.

### 6.4 Shutdown

- World unload or game quit: the mod closes with `1000`. It **MUST NOT** attempt to
  resolve in-flight migrations first — the sidecar's journal is authoritative and will
  reassert custody after the next handshake (§7.4).
- Sidecar stop: it closes with `4005` after flushing the journal.

---

## 7. Idempotency, deduplication, and replay

### 7.1 What `migrationId` is and is not

`migrationId` is the **only** idempotency key in the system (D2). It is minted once by
the origin mod and travels unchanged through both sidecars and the relay. `messageId`
identifies a *frame*, not a *migration*, and is never used for deduplication.

### 7.2 Sidecar-side deduplication

- **Outbound.** A repeated `MIGRATE_OUT` with a `migrationId` already in the journal, and
  the same payload hash, is answered with `MIGRATE_OUT_ACK` and is not journaled twice.
  A different payload hash under the same key is `DUPLICATE_MIGRATION_ID`.
- **Inbound.** A repeated Contract B `MIGRATION_PAYLOAD` with a known `migrationId` — live
  or tombstoned — is not delivered a second time. It is acknowledged upstream directly.
- **Tombstones.** A completed migration leaves a tombstone for `exportRetentionSeconds`
  (default 3600 real seconds). Tombstones are what make custody reassertion (§7.4) and
  late-retry suppression possible. They **MUST** be durable, in the same journal.

### 7.3 Mod-side deduplication — two keys, two lifetimes

The mod is stateless across connections and across world loads. It therefore needs a key
that lives in the world data, not in mod memory.

| Key | Lifetime | Used for |
|---|---|---|
| `migrationId` | In memory, for the current connection | Fast rejection of an immediate replay after a lost `MIGRATE_IN_ACK`. |
| `entityId` | **In the world itself** | The durable check. Before spawning, the mod scans `BibiteTracker.instance.bibites` for `id.id == entityId`. A hit means the organism is already here. |

The `entityId` scan is what survives a game restart. Without it, this sequence duplicates
an organism: deliver → spawn → ACK lost → game killed → world reloads from an autosave
that contains the organism → sidecar replays → second spawn.

The scan is linear over the live population and runs once per delivery, which is the same
cost the parent/child re-link already pays (`m1_findings.md` §4.3). It is not a hot path.

> **Accepted risk.** `BibiteID.id` is a random `int32` with no allocator
> (`m1_findings.md` §4.1). Collision probability is about `n / 2^32`. A collision makes
> the mod refuse a legitimate spawn and report `duplicate: true`. That is **loss, never
> duplication**, which is the direction D2 chose.

### 7.4 Session rollback and custody reassertion

A `kill -9` on the game process rolls the world back to its last autosave. That autosave
can contain an organism the sidecar has already exported. Without reconciliation, the
organism now exists in two sims.

The rule:

1. The mod mints a new `sessionId` on every world load and sends it in `CONFIG_UPDATE`.
2. When the sidecar sees a `sessionId` it has not seen before, it walks its journal and
   its tombstones for every **outbound** migration whose custody it holds or has
   completed within `exportRetentionSeconds`.
3. For each, it sends an **unsolicited `MIGRATE_OUT_ACK`** with `unsolicited: true`,
   carrying `migrationId` and `entityId`.
4. The mod resolves each one by `entityId` (§5.5, step 2) and destroys the local copy if
   the rollback resurrected it.

This uses the message's existing meaning — "custody is mine, destroy it" — and needs no
new message type. It converts a duplication into a loss, which is the trade D2 ratified.

> **Flagged for the owner.** This slightly widens `MIGRATE_OUT_ACK` beyond the one-line
> description in `system_decomposition.md`, from "answer to a `MIGRATE_OUT`" to "custody
> assertion, solicited or not". The alternative is a tenth message type, which would
> change the ratified Contract A message list. See §12, open item 2.

### 7.5 Replay of un-ACKed inbound deliveries

After the handshake and the first `EDGE_STATUS`, the sidecar replays every journaled
inbound organism that has no `MIGRATE_IN_ACK`, in journal order, with `attempt`
incremented. Replay is unconditional — the sidecar does not try to guess whether the
previous delivery landed. The mod's `entityId` dedup absorbs the difference.

---

## 8. Heartbeat and liveness

| Direction | Mechanism | Interval | Timeout |
|---|---|---|---|
| mod → sidecar | `HEARTBEAT` message | `heartbeatIntervalMs` = 1000 ms wall clock | `heartbeatTimeoutMs` = 3500 ms |
| sidecar → mod | WebSocket ping / pong frames | `wsPingIntervalMs` = 15000 ms | `wsPongTimeoutMs` = 10000 ms |

The mod's heartbeat timer runs on wall-clock time, not simulated time. A paused sim, a
`0×` time scale and a 20× time scale all produce the same heartbeat cadence.

**When heartbeats stop**, the sidecar **MUST**, in this order:

1. Close the WebSocket with `4004`.
2. Mark **every** local edge closed, and publish that over Contract B. The relay's
   liveness rules then ripple it to the neighbours' mods as `EDGE_STATUS`
   (`system_decomposition.md`, `multiverse-relay`). A dead sim must not keep receiving
   organisms.
3. **Keep custody of everything in the journal.** It **MUST NOT** bounce, drop, or expire
   an inbound organism because the mod is absent. Bounce-back is for a Contract B failure,
   not for a local one.
4. Keep accepting inbound Contract B deliveries into the journal until `inboundQueueMax`
   (default 64) un-delivered entries have accumulated, then NACK further deliveries
   upstream as overloaded. This bounds the journal against an absent mod.
5. Wait for a new connection. Replay on handshake (§7.5), and reassert outbound custody if
   the `sessionId` changed (§7.4).

The sidecar **MUST NOT** treat a `HEARTBEAT` with `paused: true` as a liveness failure,
and **MUST NOT** count a missing `MIGRATE_IN_ACK` against the mod while the last
heartbeat reported `paused: true` or `timeScale: 0`.

---

## 9. Error taxonomy

Every NACK carries a `code` and a `class`. A receiver that does not recognise a `code`
falls back to the `class`. New codes are additive within a major version, so **never**
switch on `code` without a default branch.

### 9.1 `MIGRATE_OUT_NACK` codes (sidecar → mod)

| Code | Class | Cause | Required mod reaction |
|---|---|---|---|
| `EDGE_CLOSED` | transient | The exit edge has no live neighbour, or it was closed since the last `EDGE_STATUS`. | Revive. Stop migrating on that edge until a new `EDGE_STATUS` opens it. |
| `NO_ROUTE` | transient | The edge is open but the relay has not granted a ring slot yet, so there is no east neighbour to route to. | Revive. Retry after `retryAfterMs`. |
| `SIM_SIZE_MISMATCH` | transient | The neighbour's `S` differs from `simulationSize`. The transverse mapping would be ill-defined (`m2_findings.md` §2.4). | Revive. Treat the edge as closed until a new `EDGE_STATUS`. |
| `PEER_INCOMPATIBLE` | permanent | The neighbour's game version or mod set cannot accept this organism. | Revive. Mark the edge unusable for this peer. |
| `KIND_UNSUPPORTED` | permanent | `kind` is not `"bibite"`. | Revive. Never retry this organism. |
| `INVALID_PAYLOAD` | permanent | `bb8-schema` rejected the blob, or it exceeded `maxPayloadBytes`. | Revive. Never retry this organism. Log the blob size and the first 200 characters. |
| `DUPLICATE_MIGRATION_ID` | permanent | The `migrationId` is journaled against a different payload. A mod defect. | Revive. Log loudly. Never reuse that id. |
| `RATE_LIMITED` | transient | Local outbound rate limit, or the remote's admission control refused. | Revive. Retry after `retryAfterMs`. |
| `JOURNAL_FULL` | transient | The journal is at its size or entry limit. | Revive. Retry after `retryAfterMs`. |
| `JOURNAL_ERROR` | transient | The durable write failed. Custody was **not** taken. | Revive. Retry after `retryAfterMs`. |
| `MALFORMED_MESSAGE` | permanent | The `data` object failed field validation — a bad enum, an out-of-range `exitPosition`, a `NaN`. | Revive. Log loudly. This is a mod defect. |
| `SHUTTING_DOWN` | transient | The sidecar is draining. | Revive. The connection closes with `4005` next. |

No `MIGRATE_OUT_NACK` carries an `edge` field. `EDGE_CLOSED` and `SIM_SIZE_MISMATCH` are
about one specific edge, and the mod recovers it by looking the `migrationId` up in its
own in-flight record. M2 declares exactly one edge, so the two are the same answer; a
multi-edge version needs the field (amended — §13, A5; §12 open item 6). **The ring makes
that condition permanent rather than temporary, and the debt stays open and moot**
(amended — §14, A15).

**No code in this table is ever caused by `parents`** (added — §14, A12). The lineage annex
is best-effort by construction: a missing, oversized, malformed or unhashable parent blob
is recorded as a gap and the migration proceeds. `INVALID_PAYLOAD` refers to the migrant's
own `payload` and to nothing else.

### 9.2 `MIGRATE_IN_NACK` codes (mod → sidecar)

| Code | Class | Cause | Required sidecar reaction |
|---|---|---|---|
| `SIM_NOT_READY` | transient | No world is loaded, the world is still loading, or the sim is paused. | Keep custody. Re-deliver after `retryAfterMs`. |
| `SIM_OVERLOADED` | transient | The local population is above the mod's admission ceiling. | Keep custody. Re-deliver after `retryAfterMs`. Feed this into admission control. |
| `EDGE_CLOSED` | transient | `entryEdge` is not one of the edges the mod **declared** in `CONFIG_UPDATE.borderEdges`. A declared edge that is merely closed right now still accepts inbound organisms — a bounce-back arrives exactly when the edge just closed (amended — §13, A3). | Keep custody. Do not re-deliver on this edge until `CONFIG_UPDATE` changes `borderEdges`. |
| `DESERIALIZE_FAILED` | permanent | `LoadBibiteOrEggFromData` returned `null`. The method swallows every exception, so no detail is available (`m1_findings.md` §1.2). | Do not re-deliver. Hold in the journal for an operator (amended — §13, A7). |
| `RELINK_FAILED` | permanent | The restore succeeded but the parent/child repair threw. The mod destroyed the half-restored organism before replying. | Do not re-deliver. Hold for an operator. Log loudly — this is the M1 carried gap firing (amended — §13, A7). |
| `VERSION_UNSUPPORTED` | permanent | `gameVersion` does not match the running `Application.version` and the mod refuses the risk. | Do not re-deliver. Hold for an operator. Mark the peer pair incompatible (amended — §13, A7). |
| `KIND_UNSUPPORTED` | permanent | `kind` is not `"bibite"`. | Do not re-deliver. Hold for an operator. |
| `MALFORMED_MESSAGE` | permanent | The `data` object failed field validation. A sidecar defect. | Do not re-deliver. Hold for an operator. Log loudly. |
| `SHUTTING_DOWN` | transient | The game is unloading the world or quitting. | Keep custody. Re-deliver after the next handshake. |

### 9.3 What is a NACK and what is a close

| Situation | Answer |
|---|---|
| Bad JSON, missing envelope field, wrong envelope type | Close `4003` |
| Unsupported `protocol` major version | Close `4000` |
| Unknown `type` | Ignore, log one warning, keep the connection |
| Unknown field inside `data` | Ignore silently |
| A `data` field fails validation on a type that **has** a NACK (`MIGRATE_OUT`, `MIGRATE_IN`) | The matching NACK with `MALFORMED_MESSAGE`, keep the connection |
| A `data` field fails validation on a type with **no** NACK channel (`CONFIG_UPDATE`, `HEARTBEAT`, `MIGRATE_IN_ACK`, `MIGRATE_IN_NACK`) | Close `4003` (amended — §13, A8) |
| A well-formed `MIGRATE_IN_ACK` / `MIGRATE_IN_NACK` naming an unknown `migrationId` | Log one warning and ignore. Not a close — it is a late reply after a purge or a restart (amended — §13, A8) |
| A `MIGRATE_IN` with no usable `migrationId` | Log one error and drop the frame. No NACK, no close (amended — §13, A2) |
| Frame over `maxFrameBytes` | Close `1009` or `4003` |
| `payload` over `maxPayloadBytes` | `MIGRATE_OUT_NACK` / `INVALID_PAYLOAD`, keep the connection |

---

## 10. Tunables and defaults

Both sides ship these defaults. Only the owning side needs a knob for its own values.

| Name | Default | Owner | Meaning |
|---|---|---|---|
| `port` | `8787` | both | The sidecar's listen port and the mod's connect port. The M3 rig uses `8787`, `8788` and `8789` for slots 1 to 3. |
| `heartbeatIntervalMs` | `1000` | mod | Wall-clock heartbeat cadence. |
| `heartbeatTimeoutMs` | `3500` | sidecar | Three missed heartbeats plus slack. |
| `wsPingIntervalMs` | `15000` | sidecar | WebSocket ping cadence. |
| `wsPongTimeoutMs` | `10000` | sidecar | Pong deadline. |
| `migrateOutTimeoutMs` | `5000` | mod | How long the mod waits for an ACK/NACK before it stops waiting — the organism stays inert either way (§6.3). |
| `migrateInAckTimeoutMs` | `10000` | sidecar | How long the sidecar waits for `MIGRATE_IN_ACK` before it re-queues. |
| `reconnectBackoffMinMs` | `1000` | mod | Backoff floor. |
| `reconnectBackoffMaxMs` | `30000` | mod | Backoff ceiling. |
| `stableSessionMs` | `5000` | mod | How long a connection must live before the backoff ladder resets (§6.2, §13 A8). |
| `simNotReadyGraceMs` | `2000` | mod | How long a `MIGRATE_IN` may sit queued at `timeScale` 0 before `SIM_NOT_READY` (§13, A4). |
| `simSizeEpsilon` | `1e-6` | both | Relative tolerance for every `S` comparison (§13, A10). |
| `maxFrameBytes` | `8388608` | both | 8 MiB WebSocket frame limit. |
| `maxPayloadBytes` | `4194304` | both | 4 MiB limit on the `payload` string, in UTF-8 bytes. Applies to `parents[].payload` individually as well (§14, A12). |
| `maxParentBlobs` | `2` | both | Upper bound on `parents` entries. A bibite has at most two parents; a longer array is truncated by the sidecar with one warning, never a NACK (§14, A12). |
| `frameHeadroomBytes` | `65536` | mod | Envelope, escaping and JSON overhead the mod reserves under `maxFrameBytes` when it decides whether a parent blob still fits (§5.3, §14 A12). |
| `inboundQueueMax` | `64` | sidecar | Un-delivered journal entries before inbound admission control kicks in. |
| `exportRetentionSeconds` | `3600` | sidecar | Tombstone lifetime. Bounds custody reassertion (§7.4). |
| `migrationCooldownSeconds` | `5` | mod | Simulated seconds before the same organism retries after a transient NACK, on top of `retryAfterMs`. |
| `entryImmunitySeconds` | `5` | mod | Simulated seconds after arrival during which an organism cannot re-trigger the border strip (`m2_findings.md` §4.4). |

---

## 11. Implementation notes

These notes are non-normative. They exist so the two sides do not have to negotiate.

### 11.1 For the Go implementer (`multiverse-sidecar`)

- **Two-pass decode.** Decode the envelope with the body left raw, switch on `Type`, then
  decode the raw body into the concrete struct. This is why `data` is nested rather than
  flat.

  ```go
  type Envelope struct {
      Protocol  string          `json:"protocol"`
      Type      string          `json:"type"`
      MessageID string          `json:"messageId"`
      SentAt    int64           `json:"sentAt"`
      Data      json.RawMessage `json:"data"`
  }
  ```

- **One writer.** Every mainstream Go WebSocket library forbids concurrent writes. Run
  one writer goroutine fed by a buffered channel, and one reader goroutine. Never write
  from a handler.
- **`entityId` is `int32` and often negative.** Do not use `uint32` and do not use
  `json.Number` comparisons.
- **Reject `NaN`/`Inf` explicitly, but expect the decoder to get there first.** Go's
  `encoding/json` refuses to encode them, refuses a bare `NaN` token as invalid JSON, and
  refuses an overflowing literal: `1e999` into a `float64` fails with *"cannot unmarshal
  number 1e999"*. It does **not** decode to `+Inf` (amended — §13, A9). Both therefore
  surface one layer earlier — as a malformed envelope field, or as a failed `data` decode
  that §9.3 answers. Keep the finiteness check as a second net for a value a future codec,
  a `json.Number` path, or arithmetic could still produce.
- **Durability before ACK.** `MIGRATE_OUT_ACK` is only correct after `f.Sync()` on the
  journal file **and** on its parent directory. An `os.Rename` without a directory sync is
  not durable across a power loss.
- **Tombstones are journal entries.** Do not keep them only in memory, or §7.4 breaks on
  a sidecar restart — which is exactly one of the M2 kill tests.
- **Bound the outbound queue.** If the mod stops reading, the writer channel must drop
  `HEARTBEAT`-class traffic and close the connection rather than grow without limit.
- **The clock is not a source of truth.** Never compare `sentAt` against `time.Now()` for
  anything but a log line (D5).

### 11.2 For the C# implementer (`bibites-mod`)

- **Nothing touches Unity off the main thread.** The WebSocket runs on a background
  thread or a `Task`. Inbound frames go onto a `ConcurrentQueue<string>` and are drained
  on the main thread. Outbound frames go onto a second `ConcurrentQueue<string>` that the
  socket thread drains. Instantiate, destroy and transform writes are all
  main-thread-only.
- **Drain in `Update`, act in `FixedUpdate`** (amended — §13, A4). `FixedUpdate` does not
  run at `timeScale` 0, so a mod that drains only there goes silent the moment a player
  pauses — no heartbeat, no `EDGE_STATUS` applied, no reconnect. Split the work: the
  socket queue drain, connect and disconnect handling, `EDGE_STATUS`, and the wall-clock
  `HEARTBEAT` all run in `Update`; every organism spawn, destroy and revive is deferred to
  `FixedUpdate`, which is the only place Unity physics and the game's own tick are
  coherent.
- **Never block the main thread on I/O.** No synchronous connect, no `.Result`, no
  `.Wait()`. A 5-second DNS or connect stall is a 5-second frozen simulation.
- **Parse the payload with `Newtonsoft.Json.Linq` only for the eight position numbers.**
  Use `JObject.Parse` / `ToString()`. Do not deserialize it into a typed model — that
  would re-introduce the C# schema D4 removed.
- **Subscribe, never snapshot, for `SimulationSize`.** Use
  `ScenarioIndependentSettings.Instance.SimulationSize.Subscribe(...)`. A cached `S`
  silently relocates the border strip when the value changes mid-run
  (`m2_findings.md` §2.4). Send a `CONFIG_UPDATE` with
  `reason: "sim_size_changed"` from that callback.
- **Use `SetValue`, never the `val` setter**, for any game setting the mod changes
  (`voidAvoidance`). Assigning `val` fires no event and leaves the cached statics stale
  (`m2_findings.md` §1.2). `worldWrapping` is no longer such a setting: under D10 the mod
  **reads and reports** it and never writes it (amended — §14, A13).
- **Serialize a parent only while it is alive.** `SaveSystem.SerializeBibite` on a
  destroyed GameObject is a Unity fake-null trap. Read `BibiteGenes.parent1` / `parent2`
  and let Unity's fake-null answer the liveness question: a live reference gives both the
  entity ID (`BibiteBody.id.id`) and the blob, and a fake-null one falls back to
  `parent1ID` / `parent2ID` for the ID and ships no blob — that fallback *is* the gap the
  annex records (§14, A12). Do not source the IDs from the migrant's payload, and no
  `BibiteTracker` lookup is needed: the component holds the reference already.
- **Configure from environment variables.** Both game instances share one `plugins/` DLL
  and one `config/` directory, so a BepInEx config file cannot differ per instance. The
  WSL → Windows hop needs `WSLENV` to name each variable (`dev_environment.md`).
- **Destroy correctly.** De-list from `BibiteTracker.instance.bibites` first, then
  `UnityEngine.Object.Destroy(go)`. Never `Die()` — that makes a corpse or a meat pile
  (`m1_findings.md` §5). Destruction is deferred to end of frame, so do not assume the
  object is gone in the same frame.
- **Reply to every `MIGRATE_IN`.** A silent drop turns into a re-delivery, a longer
  journal, and a slower kill test. Always answer with an ACK or a NACK.

---

## 12. Open items for the owner

1. **No authentication in M2 — and none in M3 either** (amended — §14, A17). Any local
   process can connect to the sidecar and drive migrations, and any local process can
   impersonate the sidecar to the mod. Contract A binds loopback only, and D9 keeps the mod
   and its sidecar on one machine, so the LAN milestone does not change this contract's
   exposure. The shared token M3 adds lives on **Contract B**, where the wire leaves the
   loopback (`contract-b-m3.md` §3). A bearer token here waits for M4, with TLS and the
   rest of the public-exposure set.
2. **`MIGRATE_OUT_ACK` now carries `entityId` and can arrive unsolicited** (§7.4). This
   widens the message from "answer to a `MIGRATE_OUT`" to "custody assertion". It is the
   mechanism that stops a `kill -9` on the game from duplicating an organism, and it
   avoids adding a tenth message type to the ratified Contract A list. Confirm the trade.
3. **`MIGRATE_IN` carries a normalized entry position, not absolute coordinates** (§4.3).
   Contract C's one-line description says "entry coords". The normalized form is required,
   because only the mod knows its own `S`, strip width and inset margin.
4. **Mod-side durable dedup uses `entityId`, not `migrationId`** (§7.3). Entity IDs are
   random `int32` with no allocator, so a collision suppresses a legitimate spawn. The
   result is loss, which D2 prefers over duplication, but it is a real and permanent
   accepted risk.
5. **The mod must freeze an in-flight organism** (§6.3) using `enabled = false` plus
   `rb2d.simulated = false`. That combination is inferred from the decompiled source and
   has not been confirmed in-game. If it turns out to have side effects, the fallback is
   to accept a brief live overlap during a reconnect.
6. **`MIGRATE_OUT_NACK` has no `edge` field** (§9.1, §13 A5). M2 declares one edge per
   sim, so `EDGE_CLOSED` and `SIM_SIZE_MISMATCH` are unambiguous and the mod correlates
   through `migrationId`. The first multi-edge sim needs `edge` on the NACK, which is an
   additive field and does not need a major bump. **M3 assessed and kept it open**
   (§14, A15): the ring makes one export edge a permanent property, so the debt is moot
   rather than urgent, and closing the item would erase the note the retired `{x, y}` grid
   would need if it ever returned.
7. **The capture band's direction test has no magnitude floor** (§4.3.1). `velocity.x > 0`
   admits an export on a single tick of eastward jitter, so an organism loitering in the
   band can cross without ever meaning to. M3 accepts it: a floor is a second tunable, it
   must scale with `S`, and the one-way ring bounds the consequence to "the organism moved
   on". Open because it is a **rate** question that only a populated ring can answer, and
   the answer changes a rule the mod enforces on every organism on every tick.
8. **A parent blob dropped for frame size is unreachable at the sidecar** (§14, A12). The
   sidecar records it as `"parent_gone"` because the two look identical on the wire, so
   `contract-b-m3.md` §6.6's `"blob_dropped_for_size"` is defined and never emitted. The
   fix is one additive optional field on `parents[]`; M3 does not add it.

---

## 13. Amendments (`contract-a/1`, 2026-08-02)

The Go side (`go/`) and the mod side (`bibites-mod/`) were built independently from the
text above. Each found ambiguities and resolved them locally. This section makes those
resolutions law. Every amendment names the ambiguity, the resolution, and **which side
enforces it** — the side whose code makes the rule true, and which therefore has to be
changed if the rule changes.

Where an amendment contradicts the body, the amendment wins. The body carries an
`(amended — §13, Ax)` marker at each such point.

### A1 — The mod dials only from a loaded world (§2)

**Ambiguity.** §2 says the mod connects only while a simulation world is loaded, but an
earlier smoke-test instruction had the mod dial from the main menu so a rig could be
brought up in any order.

**Resolution.** The contract wins. The mod dials after the world is ready and closes with
`1000` when the world unloads. It never holds — and never opens — a connection on the main
menu. The consequence for the sidecar is stated, not implied: **no mod connection is not
an error.** The sidecar keeps every journal entry, publishes closed edges over Contract B,
and waits (§8).

**Enforced by:** the mod. The sidecar cannot distinguish "no world loaded" from "game not
running" and must not try.

### A2 — A `MIGRATE_IN` with no usable `migrationId` is dropped (§5.7, §5.9, §9.3)

**Ambiguity.** §5.7 says the mod replies to every `MIGRATE_IN` with an ACK or a NACK, and
§11.2 repeats it. But §5.9 makes `migrationId` a REQUIRED field of the NACK, so a
`MIGRATE_IN` that arrives without one cannot be answered at all.

**Resolution.** The mod logs one error and drops the frame. No NACK, no close, no invented
id. "Without one" means absent, not a string, **or the empty string** — an empty value
fails the receiving side's own UUID check, so a reply carrying it would be answered with
close `4003`.

The sidecar's obligation is the other half: it **MUST** make the case impossible. Every
`MIGRATE_IN` is built from a journal entry, and no journal entry is created before
`migrationId` has passed a UUID check — on Contract A in `MIGRATE_OUT` validation, and on
Contract B in `MIGRATION_PAYLOAD` validation. A bounce-back reuses the original entry and
its id.

**Enforced by:** both. The mod for the drop, the sidecar for the impossibility.

### A3 — Inbound `EDGE_CLOSED` means "not a declared edge" (§9.2)

**Ambiguity.** §9.2's `EDGE_CLOSED` row gave two causes joined by "or": the mod has no
border strip on `entryEdge`, **or** it has that edge closed locally. The second reading
makes a bounce-back undeliverable, because an edge closes exactly when its peer dies —
which is the event that produced the bounce-back in the first place.

**Resolution.** Only the first cause stands. The mod answers `EDGE_CLOSED` when
`entryEdge` is not in the `borderEdges` list it declared in `CONFIG_UPDATE`. A declared
edge that is currently closed still accepts inbound organisms. Open/closed state gates
**outbound** migration only (§5.4).

**Enforced by:** the mod. The sidecar's stated reaction is unchanged and stays correct:
keep custody, and do not re-deliver on that edge until a `CONFIG_UPDATE` changes
`borderEdges`.

### A4 — `Update` carries control, `FixedUpdate` carries organisms (§8, §11.2)

**Ambiguity.** §11.2 told the mod to drain the inbound queue inside `FixedUpdate`. §8
requires the heartbeat to keep its wall-clock cadence through a pause and a `0×` time
scale. Unity does not run `FixedUpdate` at `timeScale` 0, so the two requirements cannot
both be met from one callback.

**Resolution.** Split them.

| Work | Callback | Why |
|---|---|---|
| Socket queue drain, connect/disconnect, `EDGE_STATUS`, `CONFIG_UPDATE`, `HEARTBEAT` | `Update` | Must keep running while the sim is stopped |
| Organism spawn, destroy, revive; migration timeouts; the border-strip test | `FixedUpdate` | Must be coherent with physics and the game's own tick |

A `MIGRATE_IN` therefore lands in a queue that `FixedUpdate` cannot drain while the sim is
stopped. A delivery that has waited more than `simNotReadyGraceMs` (2000 ms wall clock) at
`timeScale` 0 is answered from `Update` with `MIGRATE_IN_NACK` / `SIM_NOT_READY`,
`class: "transient"`, `retryAfterMs: 5000`, and removed from the queue.

**Enforced by:** the mod for the split and the NACK; the sidecar for the answer — it
**MUST** honour `retryAfterMs`, **MUST** keep custody, and **MUST NOT** count the silence
of a mod whose last `HEARTBEAT` reported `paused: true` or `timeScale: 0` as a missed
delivery (§8).

### A5 — `MIGRATE_OUT_NACK` carries no `edge` field (§9.1)

**Ambiguity.** `EDGE_CLOSED` and `SIM_SIZE_MISMATCH` are statements about one edge, and
§5.6's field list has no `edge`. With more than one open edge, the mod could not tell
which one to stop using.

**Resolution.** Accepted as-is for M2, which ships exactly one edge per sim. The mod
correlates the NACK through `migrationId` to its own in-flight record, which knows the
exit edge, and closes that edge until the next `EDGE_STATUS`. Recorded as §12 open item 6:
the first multi-edge sim adds `edge` to `MIGRATE_OUT_NACK`, which is an additive field and
needs no major bump.

**Enforced by:** neither, today. It is a bounded debt, not a rule.

### A6 — Bounce-back needs proof, and a timeout is not proof (§5.7)

**Ambiguity.** The custody chain's step 6 says a remote NACK **or a timeout** re-injects
the organism into the origin sim. Taken literally the timeout half duplicates: the
destination may have delivered the organism and ACKed it, and only the `MIGRATION_ACK` may
have been lost. D2 forbids duplication and prefers loss.

**Resolution.** The origin sidecar bounces in exactly two cases:

| Situation | Action |
|---|---|
| A Contract B `MIGRATION_NACK` arrived, any code | **Bounce.** `contract-b-m3.md` §6.8 forbids a NACK after durable custody, so a NACK proves custody never moved. |
| The forward never reached a live peer — relay link down, or the destination ring slot vacant — for longer than `bounceTimeoutMs` | **Bounce.** The frame was never handed to anyone. |
| The forward reached a live peer and no answer came back | **Hold and re-forward.** Never bounce. The destination deduplicates on `migrationId`, so a re-forward costs nothing and holding turns a possible duplication into a bounded delay. |

**Enforced by:** the sidecar. Already specified in `contract-b-m3.md` §9 (and in the
superseded `contract-b-m2.md` §7, where it was written first);
this amendment aligns Contract A's wording with it. The mod needs no change — a
bounce-back is an ordinary `MIGRATE_IN` with `bounceBack: true`.

### A7 — A permanent `MIGRATE_IN_NACK` is held, never returned (§5.9, §9.2)

**Ambiguity.** §5.9 offered the destination sidecar two options on a permanent NACK:
bounce the organism back to the origin peer over Contract B, or hold it for an operator.
The first option is unsafe. The payload is already durably journaled at the destination,
M2's message list has no two-phase return, and a lost return frame loses the organism
outright.

**Resolution.** Hold. The destination sidecar moves the journal entry to a `held` state,
keeps it for an operator, and logs one loud line. It never returns the organism over
Contract B in M2 or M3. `contract-b-m3.md` §9 and §13 item 2 already say this; §5.9 and the
three permanent rows of §9.2 now match.

**Enforced by:** the sidecar.

### A8 — No NACK channel means close `4003`, and the mod must back off (§9.3, §6.2)

**Ambiguity.** §9.3 answers a failed `data` validation with "the matching NACK". Four
message types have no matching NACK: `CONFIG_UPDATE`, `HEARTBEAT`, `MIGRATE_IN_ACK` and
`MIGRATE_IN_NACK`.

**Resolution, sidecar side.** An unusable frame of those four types closes the connection
with `4003`. §5.1 already sets the precedent by closing `4003` on a first frame that is
not `CONFIG_UPDATE`.

One exception: a `MIGRATE_IN_ACK` or `MIGRATE_IN_NACK` that is **well-formed** but names a
`migrationId` the sidecar does not know is logged at warning level and ignored. It is a
late reply to a delivery that has since been completed, purged or restarted through — a
normal race, not a defect, and closing on it would turn a race into an outage.

**Resolution, mod side.** Close `4003` therefore arrives *after* a successful socket
connect, on every attempt, for as long as the fault lasts. The mod's reconnect backoff
**MUST** treat that as a failure: the ladder resets only after a session that stayed up
for `stableSessionMs`, never on the connect itself, and every reconnect waits out at least
rung 1 (§6.2). Resetting on connect holds the ladder at its floor and produces an
unthrottled redial loop against a peer that is failing deterministically.

**Enforced by:** both. The sidecar for the close, the mod for surviving it.

### A9 — `1e999` does not decode to `+Inf` in Go (§11.1)

**Ambiguity.** §11.1 justified the mandatory finiteness check with the claim that `1e999`
decodes to `+Inf` in Go. It does not. `encoding/json` rejects an overflowing literal for a
`float64` target with *"cannot unmarshal number 1e999 into Go value of type float64"*, and
a bare `NaN` token is not valid JSON at all.

**Resolution.** Both faults surface one layer earlier than §11.1 assumed:

- in an envelope field, as a malformed frame → close `4003` (§3.2);
- inside `data`, as a failed body decode → the answer §9.3 gives for that type, which is
  `MALFORMED_MESSAGE` where a NACK exists and close `4003` where one does not (A8).

The finiteness check stays, as a second net rather than the first one. It still catches a
value that arrives through a `json.Number` path, from a non-Go peer, or out of arithmetic.
§4.1's ban on `NaN` and `±Inf` is unchanged.

**Enforced by:** the sidecar. Documentation-only — no behaviour changed.

### A10 — `S` equality is a relative epsilon (§4.1, §5.3, §5.4)

**Ambiguity.** §5.3 step 4 tells the sidecar to check that the neighbour's `S` **equals**
`simulationSize`, and §5.4 tells the mod to compare `peerSimulationSize` against its own
`S`. §4.1 forbids comparing two floats for exact equality — `S` starts life as a C#
32-bit `float` and reaches the sidecar as a `float64`.

**Resolution.** One test, used by both sides everywhere this contract says two `S` values
are equal:

```
equal(a, b)  ⇔  finite(a) ∧ finite(b) ∧ |a − b| ≤ 1e-6 · max(1, |a|, |b|)
```

The `max(1, …)` term keeps the tolerance absolute near zero and relative at simulation
scale: at `S = 2000` it is 2 mm of world units, far below any real resize and far above
any float round trip. A non-finite value on either side is never equal to anything.

The same function backs `MIGRATE_OUT` step 4, the `EDGE_STATUS` computation of §5.4, and
Contract B's inbound admission check.

**Enforced by:** the sidecar for the wire decisions, the mod for its own second check of
`peerSimulationSize` (§5.4).

### Amendments that live in Contract B

Three resolutions from the Go side are Contract B's business and are written there. They
are listed here so a Contract A reader is not left looking for them. All three survive M3
unchanged; the M3 text for each is in `contracts/contract-b-m3.md`, which supersedes
`contract-b-m2.md`.

| Resolution | Where it is written | Summary |
|---|---|---|
| `entityId` and `heading` travel explicitly on the Contract B wire | `contract-b-m3.md` §6.6, §11 item 3 | Additive fields. `entityId` from the blob wins over the wire value, with a warning on a mismatch, because the blob is what the destination restores. `heading` is the reverse: the wire value wins, because the receiving mod rewrites the blob from it (§5.7 step 3). |
| The shape of `$.body.id` in a bb8 blob | `contract-b-m3.md` §11 item 3 | Both `{"body":{"id":N}}` and `{"body":{"id":{"id":N}}}` are accepted. §5.3's example elides the `BibiteID` wrapper, so the two are indistinguishable from this document alone. |
| The sidecar persists what the relay granted it | `contract-b-m3.md` §7.4 | `<data-dir>/slot` and `<data-dir>/peer-id`, replayed as `preferredSlot` and `peerId` on the next claim, so a relay restart cannot swap two sims and a sidecar restart cannot lose its ring slot. In M2 this was `<data-dir>/sector`. |

---

## 14. M3 amendments (`contract-a/1.1`, 2026-08-03)

Decisions D8–D11 were ratified on 2026-08-03 and redefined M3
(`system_decomposition.md`; `m3_considerations.md`). This section carries the four items
that document's *Contract Changes Needed* table assigns to Contract A — items 6, 7, 8 and 9
— plus the three consequences that fall out of them: the retired sector, the version bump,
and the authentication question M3 does **not** answer here.

The section follows §13's pattern. Each amendment names the ambiguity or change, the
resolution, and **which side enforces it** — the side whose code makes the rule true, and
which therefore has to change if the rule changes. Where an amendment contradicts the body
or §13, the amendment wins, and the body carries an `(amended — §14, Ax)` or
`(added — §14, Ax)` marker at each such point.

Unlike §13, this set **changes the wire**. Every change is additive or narrowing, so it is
a minor bump, not a major one — see A16.

### A11 — `EDGE_STATUS` governs one export edge; the entry edge is passive (§2, §4.2, §5.1, §5.4, §5.7)

*Work order item 6 (D8).*

**Change.** M2's `EDGE_STATUS` reported one entry per declared border edge, and a declared
edge was both an exit and an entrance. D8 gives every sim exactly two doors and makes them
different doors: it **exports through its east edge** into its one east neighbour, and
**receives through its west edge**.

**Resolution.**

| Rule | Statement |
|---|---|
| `CONFIG_UPDATE.exportEdge` | New field, REQUIRED from `contract-a/1.1`. The one edge this sim may export through — `"E"` under the ring. A frame without it is not a close: a single-entry `borderEdges` supplies the value, and only an ambiguous `borderEdges` closes `4003`. That fallback is what keeps the field *additive* under §3.1's minor rule. |
| `CONFIG_UPDATE.borderEdges` | Keeps its name, narrows its meaning: the edges on which the mod has a strip and will **accept** an inbound organism. `["E", "W"]` under the ring. |
| `EDGE_STATUS.edges` | **Exactly one entry**, for `exportEdge`. An empty array closes it. Extra entries are ignored with a warning, never a close. |
| The entry edge | **Always accepts.** It is never listed, never opened, never closed, and it has no capture band (§4.3.1). |
| `edges[].reason` | Gains the value `"peer_mod_absent"`: the east neighbour's sidecar is live but has no mod, so it cannot spawn anything. |
| `MIGRATE_IN.entryEdge` | `"W"` for ring traffic; the sim's own `exportEdge` for a bounce-back. Both are declared, so §13 A3's inbound `EDGE_CLOSED` rule accepts both. |
| Entry immunity | REQUIRED, not optional. Geometry stops an ordinary re-export; only the immunity window stops a bounce-back from re-exporting on the next tick. |

The ripple is one-way, like the lanes: when a peer dies, its **west** neighbour loses its
export target and closes its export edge; its east neighbour simply receives nothing and is
told nothing (`contract-b-m3.md` §8).

**Enforced by:** the sidecar for the single-entry `EDGE_STATUS` and the reason mapping; the
mod for declaring `exportEdge`, for accepting an inbound organism on either declared edge,
and for the immunity window.

### A12 — `MIGRATE_OUT` carries opaque parent blobs; the sidecar hashes them (§5.3, §9.1, §10)

*Work order item 7 (D11, D4).*

**Ambiguity.** D11 puts a **content hash of each parent genome** in every migration
envelope. D4 forbids the mod to parse a bb8 body, and a hash of a genome projection is a
parse (`m3_considerations.md`, Risk 8). Taken together they say the mod must produce
something it is not allowed to compute.

**Resolution — split the work at the process boundary.** The mod **ships**, the sidecar
**hashes**:

1. The mod reads the parent entity IDs from the migrant's live `BibiteGenes` component —
   the `parent1` / `parent2` `GameObject` resolved to `BibiteBody.id.id` while it is alive,
   and the component's `parent1ID` / `parent2ID` when it is not. Integers either way, no
   genome parsing involved.
2. For each parent still alive locally, the mod calls `SaveSystem.SerializeBibite` and puts
   the result in `parents[].payload` as an **opaque string**, exactly as it does for the
   migrant. It never looks inside it.
3. The sidecar hashes the migrant's genome and every present parent blob with
   `bb8-schema`'s canonical projection (`contracts/genome-hash.md`), caches each blob by
   hash so it can answer a later `GENOME_REQUEST`, assembles the `LineageAnnex`, and
   **strips every parent blob** before the envelope goes on the Contract B wire.
4. A parent with no blob — dead, unserializable, or dropped for size — is recorded as a
   **gap**. `BibiteGenes` drops the parentage once the parent GameObject is gone, so a gap
   is the normal case, not a failure (D11).

D4 is intact: the mod still never parses a genome. Risk 7 is answered as a side effect —
the source sidecar holds the parent genome, so it can serve the archive's fetch.

**Why step 1 reads the component and not the blob.** `BibiteGenes.SaveState` writes
`$.genes.parent1` only `if (parent1 != null)` (`BibiteGenes.cs:552-559`), so the payload
carries a parent key **only while that parent is alive** — precisely the case where the mod
does not need the payload, because it can serialize the parent. A payload-sourced reader
would therefore never produce a gap at all, and the lineage of a departed parent would be
lost silently. The component keeps both handles: the `GameObject`, and the `[NonSerialized]`
`parent1ID` / `parent2ID` that `LoadState` fills from a payload that did carry the key.

**Contract debt — a dropped blob is indistinguishable from a dead parent.** §6.6 of
`contract-b-m3.md` defines three `gapReason` values, and the sidecar can only ever emit two
of them. A parent the mod dropped for frame size arrives on Contract A as an entry with an
`entityId` and no `payload` — byte for byte what a dead parent looks like — so the sidecar
records `"parent_gone"` and `"blob_dropped_for_size"` is unreachable. The mod logs the drop
loudly on its own side, so the information is not lost, only unjoined. **M3 adds no flag for
this.** The fix is one optional boolean on `parents[]`, it is additive, and it is worth doing
the first time an operator actually has to correlate two logs to answer "did we ship that
genome?". Recorded here so the next reader does not mistake the missing value for a defect.

The cost lands on the export path, on the Unity main thread: one export may now serialize
three organisms in one `FixedUpdate`. §5.3 requires a within-tick cache by entity ID, and
`m3_considerations.md` WP4 requires that cost to be measured before the exit test.

**Enforced by:** the mod for collecting and shipping; the sidecar for hashing, caching,
stripping and for gap-recording. Neither may take the other's half — a mod that hashes
breaks D4, and a sidecar that trusts a mod-supplied hash breaks the archive's join key.

### A13 — Export capture reaches outside the square, and the wrap stays on (§4.3.1, §5.3, §5.4, §11.2)

*Work order item 8 (D10).*

**Change.** M2 disabled `worldWrapping` while an edge was open and captured only inside the
square. Both halves are reversed.

**Resolution.**

- **The mod never writes `worldWrapping`.** It snapshots the value and reports it. The wrap
  is the containment mechanism for the three edges no strip guards (D10).
- **The capture band runs from `S − W` outward with no outer bound** (§4.3.1), so an
  organism that clears the whole strip in one tick is still captured, before the wrap can
  teleport it.
- **Every capture requires an outward velocity, everywhere in the band.** This is the test
  that separates an export from a wrap: a wrapped organism arrives in the far outer band
  travelling **inward**. Removing the test turns every wrapped arrival into a false export
  (`m3_considerations.md`, Risk 1).
- **The band is tested every `FixedUpdate`**, never on a slower timer, because the wrap
  fires from the organism's own tick.
- **`exitPosition` clamping is now load-bearing.** A diagonal escape produces a normalized
  position well outside `[0, 1]`; §4.3's existing rules — sender clamps, receiver rejects
  the unclamped — are unchanged and now actually fire.

Two in-game tests gate this amendment, and they are `m3_considerations.md` Risks 1 and 2.
Until both pass, no other M3 containment claim is evidence.

**Enforced by:** the mod. The sidecar cannot see a position and must not try; it copies
`exitPosition` and never converts it (§4.3).

### A14 — The `{x, y}` sector is retired; a ring slot replaces it (§5.1, §2.1)

*A consequence of work order item 1 (D8).*

**Change.** `CONFIG_UPDATE.sector` was an advisory `{x:int, y:int}` and a mismatch closed
the connection with `4001 SECTOR_MISMATCH`. D8 retires the grid.

**Resolution.** `sector` is retired and replaced by `ringSlot`, an OPTIONAL integer `≥ 1`.
A `contract-a/1.1` mod **MUST NOT** send `sector`; a `contract-a/1.1` sidecar **MUST**
ignore it when an older mod sends it and **MUST NOT** close on it. Close code `4001` keeps
its number and its behaviour and is now read as `SLOT_MISMATCH`: the mod's configured slot
disagrees with the slot its sidecar holds, which is a mis-wired rig and is worth one second
of diagnosis instead of an hour.

The mod still learns no topology from this. `ringSlot` is a label it was configured with,
not a routing input: it never names a neighbour, and the sidecar still decides every
destination (§1).

**Enforced by:** the sidecar, for the ignore and for the `4001` check. The mod supplies the
value or omits it.

### A15 — Contract debt A5 stays open and is permanently moot (§9.1, §12 item 6, §13 A5)

*Work order item 9 (D8).*

**Assessment.** `MIGRATE_OUT_NACK` carries no `edge` field. M2 accepted that debt because
each sim opened exactly one edge. Under the ring that condition is not a temporary property
of a two-sim rig — it is the topology: a sim exports through its east edge and through no
other, so a `MIGRATE_OUT_NACK` can refer to no other edge. The mod correlates the NACK
through `migrationId` exactly as it did in M2.

**Resolution. M3 changes nothing here, and the debt is not closed.** Closing it would
delete the note, and the note is what the retired `{x, y}` grid needs if it ever returns —
D8 keeps that grid on record as a far-future extension rather than deleting it. §13 A5 and
§12 item 6 stand as written, with this assessment attached.

**Enforced by:** neither, still. It is a recorded and re-assessed debt, not a rule.

### A16 — The protocol identifier gains a minor, and M3 is a minor bump (§3, §3.1)

*Work order item 10, reconciled (D8).*

**Change.** A11 adds a field and narrows an array; A12 adds an optional array; A14 retires
an optional field and adds another. The wire shape changed, so the identifier must change
with it — but §3.1 had only a major to change.

**Resolution.** The version segment becomes `<major>.<minor>`, parsed by splitting at the
last `/` and then at the first `.`, with a missing `.` meaning minor `0`. Compatibility
stays **major-only**; the minor is never a rejection reason. This release is
`contract-a/1.1`. The URL path stays `/contract-a/v1` and serves every `contract-a/1.x`.

**Why a minor rather than the major bump the work order proposed.** That table asks for a
major bump on both contracts, on the grounds that its items 1 and 6 remove fields. That is
true of Contract B — item 1 deletes the `["A", "B"]` sector set and changes what a sector
*is*, so `contract-b-m3.md` ships as `contract-b/2`. It is **not** true here:

| M3 change to Contract A | Kind | Needs a major? |
|---|---|---|
| `CONFIG_UPDATE.exportEdge` added | additive field | no |
| `MIGRATE_OUT.parents` added | additive optional field | no |
| `EDGE_STATUS.edges` narrowed to one entry | tighter constraint on an unchanged field | no |
| `edges[].reason` gains `"peer_mod_absent"` | additive enum **value** | no |
| `CONFIG_UPDATE.sector` retired | an OPTIONAL field that receivers now ignore | no |
| capture band, wrap, immunity window | mod-side behaviour, no wire shape | no |

No field is removed, no type changes, and no enum value is removed, which is exactly §3.1's
own test. A major bump would force a lock-step upgrade of both sides for a change that does
not require one, and it would make the M2 rig's frames unreadable for no benefit.

**Enforced by:** both, symmetrically. Each side sends `"contract-a/1.1"` and compares only
the major.

### A17 — Contract A stays unauthenticated in M3 (§2, §12 item 1)

*The M3 half of §12 open item 1 (D9).*

**Ambiguity.** §12 item 1 called a shared bearer token on the Contract A upgrade "the
obvious M3 addition". D9 then defined M3 as a LAN milestone and moved public exposure to
M4, which changes the answer.

**Resolution.** Contract A stays on `127.0.0.1` with no authentication in M3. The mod and
its sidecar always share a machine (D9), so this wire never leaves the loopback and the M3
threat model does not reach it. The token M3 does add belongs to **Contract B**, where the
wire crosses the LAN (`contract-b-m3.md` §3). A bearer token here is M4 work, alongside TLS
and the rest of the public-exposure set.

The residual risk is unchanged and accepted: any local process can drive migrations or
impersonate the sidecar. On a single-owner machine that is the same trust boundary as the
game's own save directory.

**Enforced by:** neither, deliberately. Recorded so that the next reader of §12 item 1 does
not implement a token that no milestone asked for.
