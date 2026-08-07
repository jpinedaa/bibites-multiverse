# Contract A — Mod ↔ Sidecar Wire Specification

**Version:** `contract-a/2.1`
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
**Amended:** 2026-08-05, amendment set `contract-a/2.0 + A18–A28` (§15), from the ratified
decisions D12–D16 and the work order in `m4_considerations.md`, *Contract Changes Needed*.
This set is the first **breaking** change to Contract A: `exportEdge` is replaced by
`exportEdges`, which is a field removal and a type change, so §3.1's own rule forces a
**major** bump to `contract-a/2.0` and the URL path moves to `/contract-a/v2` (§15, A23).
A18–A25 were written from the design, A26–A27 from the mod implementation, and **A28 from
the M4 pre-flight pass** — all three of the later batches are clarifying, so the identifier
stays at `contract-a/2.0`. Affected body text carries an `(amended — §15, Ax)` or
`(added — §15, Ax)` marker, and **§15 wins over the body, over §14 and over §13 wherever
they disagree.**
**Amended:** 2026-08-07, amendment set `contract-a/2.1 + A30–A33` (§16), from the owner's
ratification of **Option A — species identity travels in the migration envelope**. This set
**does** change the wire, additively: `MIGRATE_OUT` and `MIGRATE_IN` each gain one OPTIONAL
`species` object, so §3.1's own rule forces a **minor** bump to `contract-a/2.1` and the URL
path does **not** move (§16, A33). The behavioural half — resolve the species by name and
rewrite `genes.speciesID` before the restore — is entirely mod-internal. Affected body text
carries an `(amended — §16, Ax)` or `(added — §16, Ax)` marker, and **§16 wins over the body
and over §13, §14 and §15 wherever they disagree.**
**Status:** implementation-ready for M4. Derived from the ratified decisions D1–D16 in
`system_decomposition.md`, the runtime facts in `m1_findings.md`, the world-geometry and
entry-position research in `m2_findings.md`, the ring, containment and lineage designs in
`m3_considerations.md`, and the grid, healing, recovery and operations designs in
`m4_considerations.md`.
**Companion documents:** `contracts/contract-b-m4.md` (`contract-b/3.1`, sidecar ↔ relay ↔
sidecar ↔ archive) and `contracts/genome-hash.md` (`bb8-genome/1`, unchanged by M4 and
unchanged by §16 — `genes.speciesID` is excluded from the canonical projection, §4.3 there).

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

Twelve ratified constraints shape everything below. The first five are M2's, three arrived
with M3 (added — §14, A11, A12, A13), and four arrived with M4 (added — §15, A18, A19,
A20, A21):

| Decision | What it forces on this contract |
|---|---|
| **D2** — durable custody, at-most-once | `migrationId` is the idempotency key. `MIGRATE_OUT_ACK` is emitted only after a durable journal write. Both sides deduplicate. Loss is preferred over duplication. **Amended by M4:** at-most-once carries exactly one bounded exception, and it lives entirely on Contract B — an orphaned outbound entry is held while its destination is dark and bounces home at the timeout (`contract-b-m4.md` §9). Nothing on this wire changes: a bounce is still an ordinary `MIGRATE_IN` with `bounceBack: true` (§15, A25). |
| **D4** — the bb8 body is opaque to the mod | The organism payload travels as a **JSON string**, not a nested object, plus a `gameVersion` tag. All structural validation is sidecar-side, in `bb8-schema`, in both directions. **The mod never parses it for meaning** and never deserializes it into a typed model. On import it rewrites a fixed, named set of JSON paths — the eight position numbers, and now `$.genes.speciesID` — and reads nothing else out of the blob (amended — §16, A31). |
| **D3** — map-edge borders | The mod reports `exitEdge` + `exitPosition` + `velocity` + `heading`. It never reports a destination. The sidecar reports `entryEdge` + `entryPosition`; it never reports absolute world coordinates. |
| **D5** — no global clock | Every timestamp in this contract is informational. No side makes a correctness decision from another side's clock. |
| **D7** — Go sidecar | The sidecar is the WebSocket **server**. The mod is the **client**. A player starts one static binary; the game finds it. |
| **D8** — ring topology (M3) | The mod has exactly **one export edge** (east) and one **passive entry edge** (west). `EDGE_STATUS` governs the export edge only; the entry edge always accepts (§14, A11). The mod still learns no topology — not its slot, not its neighbour. **Generalized by D13**: one export edge becomes a declared *set* of export edges, and the "no topology" half is unchanged and non-negotiable. |
| **D10** — containment by the vanilla wrap (M3) | The mod **MUST NOT** disable `worldWrapping`. Export capture reaches **outside** the playable square, and every capture needs an outward velocity (§14, A13). |
| **D11** — lineage annex (M3) | `MIGRATE_OUT` carries the migrant's parent entity IDs and one **opaque** serialized blob per living parent. The sidecar hashes them; the mod never does, because D4 stands (§14, A12). |
| **D13** — the grid (M4) | The mod declares **`exportEdges`, plural** — `["E", "N"]` — and `borderEdges` becomes all four edges. `EDGE_STATUS` carries **one entry per declared export edge**, each with its own state. A second capture band runs on the north edge, a second passive entry edge runs on the south, and the **corner rule** decides which edge claims an organism inside both bands (§15, A18, A19). The mod still learns no topology: it never sees a coordinate, a neighbour or a skipped slot. |
| **D12** — route around gaps, splice in anywhere (M4) | **Nothing on this wire.** Route-around, insertion, the effective-neighbour walk and the re-route rule are Contract B's, and the mod sees only an open or closed export edge (`contract-b-m4.md` §8, §9). This is the property that keeps the mod free of topology, and M4 does not break it (§15, A25). |
| **D14** — hard-stop recovery (M4) | The mod owns a periodic world save, a save on quit and a save rotation. The wire carries one **optional** save receipt on `HEARTBEAT`, so the operator surface can name the last save of a world on another machine (§15, A21). Replay and custody reassertion (§7.4, §7.5) become tested paths rather than believed ones. |
| **D15** — operations and observability (M4) | The sidecar **paces** inbound delivery out of its journal at a maximum spawn rate per unit of **simulated** time. Pacing changes *when* an organism arrives, never *whether* it arrives, and it adds no field to this wire (§15, A20). The entry-position rule keeps mirroring `exitPosition`; arrival-position spreading is parked (§15, A25). |

---

## 2. Transport

| Property | Value |
|---|---|
| Protocol | WebSocket (RFC 6455) over plain HTTP |
| URL | `ws://127.0.0.1:{port}/contract-a/v2` (amended — §15, A23). `/contract-a/v1` is still **served**, and every connection on it is closed immediately with `4000`, so an M3 mod gets the defined loud error instead of a bare HTTP 404 |
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
| `4001` | `SLOT_MISMATCH` | sidecar | `CONFIG_UPDATE.ringSlot` disagrees with the slot the sidecar holds. A mis-wired rig. The mod **MUST NOT** reconnect automatically. Named `SECTOR_MISMATCH` in M2, over the retired `{x, y}` sector; the code and the behaviour are unchanged (amended — §14, A14). |
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
  "protocol": "contract-a/2.1",
  "type": "MIGRATE_OUT",
  "messageId": "b7d1e0c4-9f2a-4c31-8b6d-2e0a41f5c7a9",
  "sentAt": 1785693600123,
  "data": { }
}
```

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `protocol` | string | yes | Protocol identifier, major and minor version: `"contract-a/<major>.<minor>"`. This release is `"contract-a/2.1"` (amended — §16, A33; `"contract-a/2.0"` before it, §15 A23). A value with no `.` means minor `0`, so the M2 string `"contract-a/1"` reads as major 1, minor 0 (amended — §14, A16). |
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
- The URL path stays major-scoped: `/contract-a/v2` serves every `contract-a/2.x` — including
  `contract-a/2.1` (amended — §16, A33) — exactly as `/contract-a/v1` served every
  `contract-a/1.x` (amended — §15, A23). A **major** bump
  therefore moves the path, and the retired path is kept alive only to answer with close
  `4000` (§2).
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

Under the grid (D13) a sim uses **all four** values (amended — §15, A18):

| Value | Role under the grid | Role under the ring (M3) |
|---|---|---|
| `"E"` | export edge, east lane | export edge |
| `"N"` | export edge, north lane | unused |
| `"W"` | passive entry edge, receives the east lane | passive entry edge |
| `"S"` | passive entry edge, receives the north lane | unused |

The opposite-edge function is what pairs them: an organism that leaves through `E` arrives
through `W`, and one that leaves through `N` arrives through `S`. A map of one row is the
ring, and there `"N"` and `"S"` are declared but never carry traffic — the north lane has no
neighbour and its `EDGE_STATUS` entry stays closed with `no_peer`.

M3's note stands for the two values it did not use, and now applies to nothing: all four
values were kept in the enum because removing one would be a major bump, and the grid needed
them. M7's payload kinds may still not share this geometry.

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
- The **mod** computes the absolute entry point. It insets the **fixed** coordinate by
  `W + margin` **inward** from the edge, and takes the **free** coordinate from the inverse
  table above (`m2_findings.md` §4.4, §(c)). All four edges are now reachable, because the
  grid gives a sim two entry edges and a bounce-back comes home through an export edge
  (added — §15, A19):

  | Entry edge | Fixed coordinate of the spawn | Free coordinate | When it happens |
  |---|---|---|---|
  | `W` | `x = −S + inset` | `y = clamp(2S · position − S)` | Ordinary traffic off the east lane |
  | `S` | `y = −S + inset` | `x = clamp(2S · position − S)` | Ordinary traffic off the north lane |
  | `E` | `x = +S − inset` | `y = clamp(2S · position − S)` | A bounce-back of an east export |
  | `N` | `y = +S − inset` | `x = clamp(2S · position − S)` | A bounce-back of a north export |

  `margin` is `entryMargin` (§10) and `inset` is `W + margin`. **The free coordinate is
  clamped to `[−(S − inset), +(S − inset)]` — the same inset the fixed coordinate takes**
  (added — §15, A28). Without that clamp a `position` near `0` or `1` would put a corner
  arrival inside the strip of the two edges perpendicular to its entry edge; with it, an
  arrival is inset from **every** edge, not only the one it came through.

  Two properties follow from the inset, and both are guarantees rather than accidents
  (amended — §15, A28):

  - **No arrival of any kind lands inside a capture band.** A band begins at `S − W`
    (§4.3.1), and every spawn coordinate — fixed and free, on either axis — is at most
    `S − W − margin` from the origin, with `margin ≥ 5` world units (§10). This holds for
    a bounce-back on its own export edge and for both axes of an ordinary arrival.
  - **The entry-immunity window is still REQUIRED, and the geometry is not a substitute
    for it.** The inset buys one tick, not a policy. A bounce-back arrives moving outward
    by construction (§5.7, `contract-b-m4.md` §9.4) and reaches `S − W` under its own
    power within a few `FixedUpdate`s, and an ordinary arrival near a corner can do the
    same on the other axis. The window is what keeps the spawn and the next export two
    separable events instead of one (§5.7 step 6, §14 A11).

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

A capture band is therefore **half-open and outward-unbounded**. There is **one band per
declared export edge** (amended — §15, A19) — `E` and `N` under the grid, `E` alone under a
one-row map:

```
inBand(organism, E)  ⇔  x ≥ S − W          the band starts at the strip line
                     ∧  velocity.x > 0     and it must be leaving

inBand(organism, N)  ⇔  y ≥ S − W          the same rule, on the other axis
                     ∧  velocity.y > 0

capture(organism)    ⇔  ∃ e ∈ exportEdges : inBand(organism, e) ∧ open(e)
                     ∧  not already in flight
                     ∧  entry immunity has expired
```

| Rule | Value |
|---|---|
| Inner boundary | `x = S − W` on `E`, `y = S − W` on `N`, with `W` = `borderWidth` |
| Outer boundary | **none.** The band runs from the strip line outward, past `S`, past the wrap radius, to any coordinate |
| Direction test | `velocity.x > 0` on `E`, `velocity.y > 0` on `N`. **REQUIRED everywhere in the band, not only outside the square** |
| Test cadence | Every `FixedUpdate`, on every live organism. **Never** on a slower timer |
| Which edge | Exactly one, chosen by §4.3.2. An organism **MUST NOT** produce two `MIGRATE_OUT`s |
| Which edges exist | The members of `CONFIG_UPDATE.exportEdges` (§5.1). An entry edge never has a band |

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

The entry edges `W` and `S` have **no capture band at all** (amended — §15, A19). They are
passive: an organism that walks west or south out of the square is not a migrant, and the
wrap returns it (D10, §14 A11, D13).

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

### 4.3.2 The corner rule — which edge claims an organism in two bands (added — §15, A19; generalized — §15, A26)

The two bands **overlap** in the north-east corner, `x ≥ S − W ∧ y ≥ S − W`. An organism
there with both velocity components outward is in both bands at once, and something has to
choose. Nothing in the geometry answers it, so the contract does: **two mods that answer
differently produce two different maps out of one world** (`m4_considerations.md`,
Question 6).

The rule, evaluated **in this order**, every `FixedUpdate`, for one organism. It is stated
for an arbitrary declared set, because the mod implements it that way and a rule written only
for `{E, N}` would leave the next export set undefined (A26):

```
outward(v, e) = v · n(e),   n(E) = (1,0)   n(N) = (0,1)
                            n(W) = (−1,0)  n(S) = (0,−1)

candidates = { e ∈ exportEdges : inBand(organism, e) ∧ outward(v, e) > 0 ∧ open(e) }

|candidates| = 0   →  no export this tick
|candidates| = 1   →  that edge
|candidates| ≥ 2   →  the candidate with the largest outward(v, e);
                      on an exact tie, the earlier edge in canonical order E, N, W, S
```

Under the grid `exportEdges` is `["E", "N"]`, both components are positive inside the corner,
so `outward(v, E)` is `velocity.x` and `outward(v, N)` is `velocity.y` — and the general rule
reduces to **`"E"` when `velocity.x ≥ velocity.y`, otherwise `"N"`**, which is the form this
section carried before A26 and the form to reason about on the M4 rig.

| Rule | Statement |
|---|---|
| Openness first | An edge that `EDGE_STATUS` reports closed is **not** a candidate. An organism in both bands with only the north lane open exports **north**, and is never pinned against a closed corner while a live lane exists. |
| The larger outward component wins | The projection is onto each edge's own outward normal, so an inward component scores negative and §4.3.1's direction test is the same comparison. Inside the north-east corner both projections are positive, and the comparison is between two positive floats with no absolute value. |
| The earlier edge takes the tie | Canonical order is `E, N, W, S`. The mod sorts its declared set into that order once, at configuration, and keeps the incumbent on an exact tie — so the tie needs no separate branch and the whole rule is one comparison. On `{E, N}` that is exactly `velocity.x ≥ velocity.y` selecting `E`. |
| **No epsilon** | An exact `≥` on two `float`s is deliberate. A tie is not a physical event — it is a determinism requirement — and an epsilon would only widen the window in which two implementations must agree on a *third* rule. Both operands are the same mod's own floats from one `Rigidbody2D` read, so there is no cross-machine float question here. This is the one place in this contract where an exact float comparison is correct, and §4.1's ban is on *equality* tests between two independently-derived values. |
| One migration | The chosen edge becomes `MIGRATE_OUT.exitEdge`, and the organism gets exactly one `migrationId` (§6.3). A mod that emits two frames for one organism in a corner is defective. |
| `exitPosition` | Computed for the **chosen** edge by §4.3, then clamped. In the corner the raw value on either axis is routinely outside `[0, 1]`, exactly as §4.3.1 describes. |

**Why the larger component and not, say, the nearer edge.** The velocity is what the
organism is doing; the position is where it happens to be. An organism sprinting east while
grazing the north strip should leave east, and it is the direction of travel that makes the
receiving world's arrival — same velocity, same heading, one continuous world (D3, §4.4) —
look like a continuation rather than a right-angle turn.

### 4.4 Velocity and heading

| Field | Frame of reference |
|---|---|
| `velocity` | `{"x": float, "y": float}`, world units per **simulated** second, in the source sim's world axes. Read from `Rigidbody2D.linearVelocity`. |
| `heading` | float, **degrees**, counter-clockwise, `0` = the `+y` axis. This is `transform.localRotation.eulerAngles.z` and `rb2d.r`, which the game keeps equal. The organism's forward direction is `transform.up` (`m2_findings.md` §1.1). |

**Velocity and heading are copied, never mirrored.** An organism that leaves eastward
enters the next slot still travelling eastward. That continuity is the whole point of
D3's map-edge model. The sidecar **MUST NOT** negate, rotate, or reflect either value in
M2, and **MUST NOT** in M3 either, because every ring slot is a pure translation of every
other (amended — §14, A11). **The grid does not change this** (amended — §15, A18): a north
hop is a translation along `y` exactly as an east hop is a translation along `x`, so an
organism that leaves northward enters the world above it still travelling northward. Neither
axis ever mirrors, and the two axes never swap.

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

`"bibite"` is the only value M2, M3 and M4 accept. `"corpse"`, `"pellet"` and `"egg"` are
reserved for **M7** (Contract C, §6.4 of the research) — the milestone renumbering of D9
moved them one place out, and D16 moved them one place further (amended — §15, A24). A
receiver that reads a reserved-but-unsupported kind answers with the NACK code
`KIND_UNSUPPORTED`. It does not close.

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
- The importing mod performs two **keyed rewrites** on this string before it restores it, and
  they are the only writes any mod makes into a blob: the eight position numbers, and
  `$.genes.speciesID` (§5.7 step 3, added — §16, A31). Both address a fixed path by name, both
  are writes, and neither reads a value back or infers anything from the blob's shape — which
  is what keeps D4 intact. Anything beyond a named path is parsing, and parsing is the
  sidecar's.

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
| `borderEdges` | array of edge enum | yes | The edges on which the mod has a border strip and can therefore **accept an inbound organism**. Under the grid it is exactly `["E", "N", "W", "S"]`: two export edges and two passive entry edges (amended — §15, A18). It does not mean "can migrate out" — `exportEdges` says that. The sidecar **MUST NOT** deliver a `MIGRATE_IN` on an edge absent from this list (§9.2, `EDGE_CLOSED`, §13 A3). |
| `exportEdges` | array of edge enum | yes | **The edges this sim may export through** (amended — §15, A18; replaces `exportEdge`). `["E", "N"]` under the grid (D13). Order is not significant and a receiver **MUST NOT** read one into it. Every member **MUST** be a member of `borderEdges`, the array **MUST** hold at least one entry, and it **MUST NOT** hold a duplicate; a frame that breaks any of those is unusable and closes with `4003` (§13, A8). The sidecar **MUST NOT** open an edge that is not in this list, whatever `borderEdges` says. Declaring an edge is a statement about **geometry, not topology**: it means "I run a capture band here", and whether that edge has a lane is the sidecar's answer in `EDGE_STATUS` (§5.4). |
| ~~`exportEdge`~~ | edge enum | — | **Removed** (amended — §15, A18). This is the field removal that makes M4 a major bump. A `contract-a/2.0` mod **MUST NOT** send it, and a `contract-a/2.0` sidecar **MUST** ignore it if it appears. There is **no** singular fallback and no inference from a one-entry `borderEdges`: a peer that speaks `contract-a/1.x` is rejected at the version check (§3.1, close `4000`) long before a field-level fallback could matter, so a fallback would only add a path that no correct peer can reach (§15, A23). |
| `borderWidth` | float | yes | `W`, the strip width in world units, and the inner boundary of the capture band (§4.3.1). Informational for the sidecar; the mod owns the geometry. |
| `ringSlot` | number (int) | no | The mod's configured slot number, from its environment. Advisory, `≥ 1`. When present and it disagrees with the slot the sidecar holds, the sidecar **MUST** close with `4001`. This catches a mis-wired rig in one second instead of one hour (added — §14, A14). **It keeps its name and its meaning under the grid** (amended — §15, A25): a slot number is the routing address on both maps, and the mod is **never** told its coordinate. There is no `position` field here and there will not be one — a position is topology, and topology is the sidecar's (D8, D13). |
| ~~`sector`~~ | object `{x:int, y:int}` | no | **Retired** (amended — §14, A14). D8 retires the `{x, y}` grid. A `contract-a/1.1` mod **MUST NOT** send it; a `contract-a/1.1` sidecar **MUST** ignore it if an older mod does, and **MUST NOT** close on it. Replaced by `ringSlot`. |
| `worldName` | string | no | Cosmetic. Often empty for a world the game itself saved. Never used as an identifier. |

**Receiver obligations.** On the handshake frame the sidecar validates `gameVersion`, then
`ringSlot`, then `exportEdges ⊆ borderEdges` with at least one member and no duplicate, then
sends exactly one `EDGE_STATUS` carrying **one entry for each member of `exportEdges`**
(§5.4). On a later `CONFIG_UPDATE` that changes `simulationSize`, `borderEdges` or
`exportEdges`, the sidecar re-validates peer agreement and sends a new `EDGE_STATUS`
whenever any edge's result changed (amended — §14, A11, A14; §15, A18).

The sidecar also **MUST** forward the declared `exportEdges` to the relay on its next
`SECTOR_CLAIM` (`contract-b-m4.md` §6.3), because the relay computes one effective
neighbour per export edge and cannot do that for an edge it has not been told about.

```json
{
  "protocol": "contract-a/2.1",
  "type": "CONFIG_UPDATE",
  "messageId": "1c2fbe80-5a17-4a2b-9a20-3d54f1b7e001",
  "sentAt": 1785693598004,
  "data": {
    "sessionId": "9a4c1e77-0b3d-4f52-8c19-6d2e7f0a5b31",
    "reason": "connect",
    "gameVersion": "0.6.3.1",
    "modVersion": "0.2.0",
    "simulationSize": 2000.0,
    "borderEdges": ["E", "N", "W", "S"],
    "exportEdges": ["E", "N"],
    "borderWidth": 60.0,
    "ringSlot": 5,
    "worldName": "M4-Slot5"
  }
}
```

A world on a **one-row map** sends the same frame with the same four `borderEdges` and the
same two `exportEdges`. It is the map, not the mod, that decides the north lane has no
neighbour, and the mod learns that as one closed edge in `EDGE_STATUS` (§5.4). A mod
**MUST NOT** vary its declaration by map shape — it has no way to know the shape, and the
declaration is about the bands it runs.

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
| `pendingIn` | number (int) | no | How many `MIGRATE_IN`s are queued in the mod but not yet spawned — **including frames still unparsed in the transport queue** under §5.7's ingest budget, so the sidecar's pacing sees the whole backlog, not the parsed part of it (amended — §15, A29). |
| `lastSave` | object | no | **The most recent periodic world save** (added — §15, A21). Absent until the mod has written one, and absent forever from a mod that does not save. It is a **receipt, not a request**: the sidecar never asks for a save and never schedules one. |
| `lastSave.atMs` | `timestampMs` | yes | The mod's wall clock when the save completed. Informational (D5) — it is a label for an operator, never an input to a decision. |
| `lastSave.simulatedTime` | float | yes | `TimeKeeper.simulatedTime` at the save. This is the value that says how much world the last save holds. |
| `lastSave.population` | number (int) | yes | Live organism count written into that save. |
| `lastSave.name` | string | no | The save's file name, for the operator who has to find it. |
| `lastSave.bytes` | number (int64) | no | Its size on disk. A collapsing byte count is the cheapest early warning of a broken save path. |
| `lastSave.durationMs` | number (int) | no | How long the save stalled the simulation. The owner's budget is **2000 ms** (D14), and this is the field that proves it at six instances. |

**Receiver obligations.** The sidecar records the arrival time and the counters. It sends
nothing back.

`lastSave` is the only wire path the operator surface has to a world's save state, and that
is why it exists (added — §15, A21). The archive serves the status page from the relay's
ring view (`contract-b-m4.md` §6.5), and a sidecar on the **second machine** has no file the
archive can read. Without this field the page reports "unknown" for every far-end world,
which Risk 4 permits but the M4 exit test does not. It is OPTIONAL, so a mod that omits it
is conformant and simply reads as unknown — an honest gap, never a zero.

```json
{
  "protocol": "contract-a/2.1",
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
    "timeScale": 20.0,
    "simulationSize": 2000.0,
    "inFlightOut": 0,
    "pendingIn": 3,
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
```

---

### 5.3 `MIGRATE_OUT` — mod → sidecar

**When sent.** An organism is inside the **capture band** of one of the mod's `exportEdges`,
that edge is **open**, the organism is moving outward, it is not already in flight, and its
entry immunity has expired. §4.3.1 states the bands and the direction test in full and the
band reaches outside the playable square (amended — §14, A13); §4.3.2 states which edge
claims an organism that is inside both bands (amended — §15, A19). The mod mints the
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
| `species` | object | no | **The migrant's species identity** (added — §16, A30). Read from the live organism's own `Species` record — `BibiteGenes.species` — never from the payload, which carries only a world-local integer. Absent when the organism has no species record, and absent from a mod that does not implement §16 at all; both are conformant, and §5.7's absent-block rule states exactly what the importer then does. |
| `species.genericName` | string | yes | The genus half of the name, byte for byte as the source world holds it (`Species.genericName`). REQUIRED when `species` is present. Non-empty, at most **64 UTF-8 bytes**, no leading or trailing whitespace. |
| `species.specificName` | string | yes | The specific half (`Species.specificName`). Same rules. The name the importer matches on is `genericName + " " + specificName`, assembled with exactly one U+0020 — the game's own `Species.name` (`Species.cs:85`). |
| `species.parentGenericName` | string | no | The genus half of the **immediate parent species'** name, taken from `Species.parentSpecies` when the migrant's species has one. Same string rules. |
| `species.parentSpecificName` | string | no | The specific half of the same name. **The two parent fields are all-or-nothing** — both present or both absent — and a block carrying exactly one of them is malformed (§16, A30). Exactly **one** generation travels: a grandparent never does, and the importer needs none, because a chain rebuilds one link at a time as its members migrate. |
| `exitEdge` | edge enum | yes | Which of the mod's own edges the organism crossed. **MUST** be a member of the `exportEdges` the mod declared, and **MUST** be the edge §4.3.2 chose (amended — §15, A18). `"E"` or `"N"` under the grid. The mod **MUST** record it against its own in-flight record: it is what makes a `MIGRATE_OUT_NACK` with no `edge` field unambiguous under two export edges (§9.1, §15 A22). |
| `exitPosition` | float | yes | `[0,1]` along that edge, by the formula in §4.3. |
| `velocity` | object `{x,y}` | yes | World velocity at the moment of capture (§4.4). |
| `heading` | float | yes | Degrees (§4.4). |
| `simulationSize` | float | yes | `S` at the moment of capture. The sidecar uses it to refuse a transfer to a peer with a different `S`. |
| `simTick` | number (int64) | yes | Informational. |

**Receiver obligations, in this order.** The sidecar **MUST**:

1. Validate the frame's fields, then validate `payload` with `bb8-schema` against
   `gameVersion`. On failure reply `MIGRATE_OUT_NACK` / `INVALID_PAYLOAD`. **The `species`
   block is schema-validated and nothing more** (added — §16, A30): a block that breaks the
   shape rules above is **stripped, logged once, and the migration proceeds without it** —
   never a NACK, and never a reason to hold an organism. The sidecar reads no meaning out of
   the two names it carries, and it **MUST NOT** synthesize, translate, normalize or
   case-fold them.
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
   (`contract-b-m4.md` §6.6). Never before step 6.

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

**Sender obligations for `species`** (added — §16, A30). The mod **MUST**:

- read the two names from the migrant's **live `Species` record**, `BibiteGenes.species`, on
  the main thread in the same `FixedUpdate` as the migrant's own serialization. The payload's
  `$.genes.speciesID` is a world-local counter value and says nothing a receiver can use, so
  it is **never** the source of this block;
- omit the whole block when `BibiteGenes.species` is `null`. A null record is a normal state —
  `ResumeBody` fills it through `CheckNewSpecies` after a restore (`BibiteBody.cs:477-480`) —
  and an omitted block is valid, never an error;
- fill `parentGenericName` and `parentSpecificName` from `Species.parentSpecies` when that
  reference is non-null, and omit **both** when it is null. A root species has no parent and
  travels without one;
- copy the names verbatim: no trimming, no case folding, no Unicode normalization, no
  re-generation. The importer's match is a byte comparison (§5.7), so any tidying on this side
  is a silent mismatch on the other;
- send the block on **every** hop, not only the first. It is read from the live record each
  time, so an organism that speciated between hops carries its new name on the next one.

The mod **MUST NOT** send a species block for a parent in `parents[]`, and there is no field
for one. The annex is about genomes and this block is about the migrant's own name; a parent's
species travels when that parent migrates, and not before.

```json
{
  "protocol": "contract-a/2.1",
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
    "species": {
      "genericName": "Cyanea",
      "specificName": "velox",
      "parentGenericName": "Cyanea",
      "parentSpecificName": "prima"
    },
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

**A north export, taken out of the corner** (added — §15, A19). The same organism one tick
earlier, at `x = 2380.4, y = 2015.7` with `vx = 61.2, vy = 88.9`, sits in both bands: it is
past `S − W = 1940` on both axes and both components are outward. §4.3.2 compares the two
and `vy > vx`, so the export is **north** and `exitPosition` is computed along `x`:
`(2380.4 + 2000) / 4000 = 1.095`, clamped to `1.0`. Only the four routing fields differ from
the frame above; `payload`, `parents`, `species`, `velocity`, `heading` and `simulationSize`
are unchanged.

```json
{
  "protocol": "contract-a/2.1",
  "type": "MIGRATE_OUT",
  "messageId": "f4b83c17-0d95-4a6e-b721-8c50a9f3e264",
  "sentAt": 1785693600123,
  "data": {
    "migrationId": "7c2e5a90-4b18-4d63-9f07-2ab6c81d4e35",
    "entityId": -843827577,
    "kind": "bibite",
    "gameVersion": "0.6.3.1",
    "payload": "{\"transform\":{ ... },\"rb2d\":{ ... },\"genes\":{ ... },\"body\":{\"id\":-843827577, ... },\"clock\":{ ... },\"brain\":{ ... },\"version\":\"0.6.3.1\"}",
    "parents": [],
    "species": {
      "genericName": "Cyanea",
      "specificName": "velox",
      "parentGenericName": "Cyanea",
      "parentSpecificName": "prima"
    },
    "exitEdge": "N",
    "exitPosition": 1.0,
    "velocity": { "x": 61.2, "y": 88.9 },
    "heading": 34.55,
    "simulationSize": 2000.0,
    "simTick": 4820343
  }
}
```

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
change to **any** export edge: a neighbour connecting or dying, an insertion, a release, a
handover, a re-pairing around a dark slot, an `S` disagreement, or an operator closing an
edge (amended — §14, A11; §15, A18).

`EDGE_STATUS` is **full state, not a delta**. Under the grid it carries **one entry for each
edge in the mod's declared `exportEdges`** — two under D13, one on a mod that declares one
(amended — §15, A18). The entry edges stay passive: they always accept an inbound organism
and never appear here.

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `epoch` | number (int64) | yes | Strictly increasing per connection, starting at 1. The mod **MUST** ignore an `EDGE_STATUS` whose `epoch` is lower than or equal to the last one it applied. This makes the message order-independent and replay-safe. The counter resets on a new connection. **One epoch covers the whole frame**, not one edge: because the frame is full state, a change on either edge raises the same counter (amended — §15, A18). |
| `edges` | array of object | yes | **One entry per declared export edge** (amended — §15, A18). At most one entry per `edge` value; a duplicate `edge` is a sidecar defect and the mod applies the **first** and logs one warning. An empty array closes **every** export edge and is the correct frame to send when the sidecar holds no slot. |
| `edges[].edge` | edge enum | yes | Which edge. **MUST** be a member of the `exportEdges` the mod declared. |
| `edges[].open` | bool | yes | `true` means migration out of **this** edge is permitted right now. The two edges are independent: a peer with a dead row still exports north (`m4_considerations.md`, Question 6). |
| `edges[].reason` | string enum | yes | Why, for this edge. When `open` is `true`: `"peer_live"`. When `open` is `false`: `"no_peer"`, `"peer_mod_absent"` (added — §14, A11), `"peer_incompatible"`, `"peer_unreachable"`, `"peer_overloaded"`, `"admin_closed"`, `"sim_size_mismatch"`. Under route-around a closed edge means **no slot on that axis is deliverable**, so the reason is an aggregate: `contract-b-m4.md` §8 fixes exactly which value the sidecar picks. |
| `edges[].peerSimulationSize` | float | no | Present when `open` is `true`. The **effective neighbour's** `S` on that edge — which may differ between the two edges, because they are different peers. The mod **MUST** compare it against its own `S` and treat that edge as closed on a mismatch, even though the sidecar already checked. Two independent checks, because a mid-run resize can race. |

`contract-b-m4.md` §8 gives the exact mapping from the relay's `PEER_STATUS` and
`SECTOR_GRANT` to these `open`/`reason` values, per axis. It is the sidecar's decision alone;
the mod never sees a peer, a coordinate, or a skipped slot.

**Receiver obligations.** Until the mod has applied its first `EDGE_STATUS`, **every export
edge is closed** and the mod **MUST NOT** send `MIGRATE_OUT`. The same holds while
disconnected. This is the fail-safe: a mod that cannot reach its sidecar quietly stops
migrating instead of losing organisms.

Three rules make the frame unambiguous, and all three follow from "full state" (amended —
§15, A18):

- **A declared export edge with no entry in the frame is CLOSED.** Absence is not "no
  change" — the frame is the whole state. This is the fail-safe direction, and it is why an
  empty array is a legal way to close everything.
- **An entry for an edge the mod did not declare is ignored**, with one logged warning and
  no close. It is a forward-compatible shape under a later map extension, not a fault. This
  rule is unchanged from §14 A11; only the criterion moved from "my one export edge" to "my
  declared set".
- **The mod applies the whole frame atomically.** It **MUST NOT** apply one entry and defer
  another, because a capture decision in the corner reads both edges in one `FixedUpdate`
  (§4.3.2) and a half-applied frame can export an organism through an edge that just closed.

The mod also uses each edge's open/closed state to drive its behaviour **at that edge** —
void-avoidance scoping (`m2_findings.md` §1.5) and the portal strip (`m4_portal_findings.md`,
WP7), both of which are now per-edge. It **no longer** drives a `worldWrapping` override:
under D10 the wrap stays ON at all times and the mod only snapshots and reports the setting
(amended — §14, A13). The `m2_findings.md` §3 advice to disable the wrap while an edge is
open is superseded.

Both lanes open, which is the ordinary state of a peer on a live grid:

```json
{
  "protocol": "contract-a/2.1",
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
      },
      {
        "edge": "N",
        "open": true,
        "reason": "peer_live",
        "peerSimulationSize": 2000.0
      }
    ]
  }
}
```

The same peer one second after the only other world in its column went dark. The east lane
re-paired around the gap in its own row and never closed; the north lane has nowhere left to
go, because a column of two holds no third slot to skip to (`contract-b-m4.md` §2,
*Degenerate shapes*). This frame, not a silence, is how the mod learns it:

```json
{
  "protocol": "contract-a/2.1",
  "type": "EDGE_STATUS",
  "messageId": "b41c7d29-6e05-4f3a-8d17-92c0be5a4f38",
  "sentAt": 1785693731660,
  "data": {
    "epoch": 2,
    "edges": [
      {
        "edge": "E",
        "open": true,
        "reason": "peer_live",
        "peerSimulationSize": 2000.0
      },
      {
        "edge": "N",
        "open": false,
        "reason": "no_peer"
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
  "protocol": "contract-a/2.1",
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
  "protocol": "contract-a/2.1",
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

**Delivery is paced** (added — §15, A20). The sidecar releases journal entries at a maximum
rate per unit of the receiving world's **simulated** time (§7.5). Pacing changes *when* an
organism arrives, never *whether* it arrives, and never its order. The mod needs no change
and gets no new field: a slower arrival stream is not a fault, not a stall, and never
something to infer anything from.

| Field | JSON type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The idempotency key. Preserved end to end from the originating mod. |
| `entityId` | `entityId` | yes | Extracted from the blob by `bb8-schema`. The mod uses it as its **durable** dedup key (§7.3) and never parses the blob itself (D4). |
| `kind` | string enum | yes | `"bibite"` in M2 and M3. Unknown kind → `MIGRATE_IN_NACK` / `KIND_UNSUPPORTED`. |
| `gameVersion` | string | yes | The version the blob is valid for, after any sidecar-side conversion. The mod compares it against `Application.version`. |
| `payload` | string | yes | The opaque bb8 blob (§4.6), already validated by `bb8-schema`. |
| `species` | object | no | **The migrant's species identity, as its origin world named it** (added — §16, A30). Same four fields, same rules and the same all-or-nothing parent pair as `MIGRATE_OUT` (§5.3); the sidecar copies it out of the envelope unchanged (`contract-b-m4.md` §6.6) and never authors one. Absent means the sender did not carry one — a legacy mod, an organism with no species record, or a block the schema check stripped — and step 3's **absent-block rule** covers all three identically. |
| `entryEdge` | edge enum | yes | The edge of the **receiving** sim the organism enters through. The sidecar computes it; the mod does not derive it. Under the grid it is the **opposite of the sender's `exitEdge`** — `"W"` for traffic off an east lane, `"S"` for traffic off a north lane — and, for a bounce-back, the **origin's own exit edge**, because a bounce-back comes home through the door it left by (amended — §15, A18; §14, A11; `contract-b-m4.md` §6.6, §9). All four values therefore appear here in M4. |
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

**The queue drains under a budget, strictly in order** (added — §15, A29). The mod applies
at most a few deliveries per `FixedUpdate` (a wall-clock budget of ~8 ms or 4 spawns,
whichever trips first, and always at least one) and parses at most a handful of frames per
`Update` (~2 ms or 16 frames); the remainder waits, in arrival order, and `pendingIn`
(§5.2) reports it. A budget stops early, it never skips or reorders. This is what keeps
`Update` — and therefore the §8 heartbeat — running through any burst: the slot-6 livelock
was a 64-deep replay applied in one `FixedUpdate`, which starved the heartbeat past
`heartbeatTimeoutMs` and turned one stall into a permanent reconnect loop.

Processing order:

1. Deduplicate (§7.3). On a hit, reply `MIGRATE_IN_ACK` with `duplicate: true` and spawn
   nothing.
2. Compute the world entry point from `entryEdge`, `entryPosition`, its own `S`, its own
   strip width and inset margin (§4.3, `m2_findings.md` §(c)).
3. **Resolve the species, rewrite the payload, then restore.** All three on the Unity main
   thread, and all three **before** the game's own deserializer runs: `BibiteGenes.LoadState`
   binds the species from the blob it is handed (`BibiteGenes.cs:581-583`), and nothing after
   the call can correct that without moving a live organism between species
   (added — §16, A31).

   **(a) Resolve.** When a `species` block is present, assemble
   `name = genericName + " " + specificName` and look it up in the local
   `GlobalLineageManager.recordedSpecies` by **exact, ordinal, case-sensitive** string
   equality against `Species.name`. The registry searched is the full recorded one, not the
   active subset.

   | Outcome | What the mod does |
   |---|---|
   | Exactly one match | That local species is the answer. If it is not in `activeSpecies` it is re-activated, exactly as `GlobalLineageManager.FindSpeciesFromTemplate` re-activates one (`GlobalLineageManager.cs:277-300`). **A migrant reviving a locally extinct species of the same name is the correct outcome**, not the defect §16 closes: the name is the same species, and the species genuinely came back. |
   | More than one match | Take the **first** in `recordedSpecies` order and log one warning. The game's own generator name-checks against `recordedSpecies` before it issues a name (`SpeciesNameGenerator.RandomUnusedGenus` / `RandomUnusedSpecific`), so a within-world duplicate should not exist; this branch is here to be deterministic, not because it is expected. |
   | No match | **Create** one local species whose `name` is exactly `name`, with both halves taken from the block and **never** re-generated — the game's `Species(BibiteBody, parent)` constructor calls `GenerateNewName()`, and a mod that lets it stand has thrown the identity away. When the block carries a parent name **and** a local species matches that name exactly, the new species is created as that species' **child** and inserted after it in the registry, the way `CreateNewSpecies(bibite, parentSpecies)` does (`GlobalLineageManager.cs:317-339`). Otherwise it is created as a **root**. |

   **(b) Rewrite `$.genes.speciesID`** in the payload JSON to the resolved or created
   species' own **local** id — a JSON integer; `Species.speciesID` is an `int64`
   (`Species.cs:22`). With no `species` block, **remove the key** instead: the absent-block
   rule below.

   **(c) Rewrite the eight position numbers** in the payload JSON —
   `$.transform.position[0]`, `$.transform.position[1]`, `$.transform.rotation`,
   `$.rb2d.px`, `$.rb2d.py`, `$.rb2d.vx`, `$.rb2d.vy`, `$.rb2d.r` — then call
   `SaveSystem.instance.LoadBibiteOrEggFromData(json, true, null, null)`.
4. **In the same frame**, re-assert `transform.position`, `transform.rotation`,
   `rb.position`, `rb.linearVelocity` and `rb.rotation` directly. The `Rigidbody2D` wins
   over the transform on the next tick, and the parent transform of `bibiteHolder` is
   still unproven (`m2_findings.md` §4.3).
5. Repair `genes.parent1` / `parent2` and any parent's `eggLayer.children`, mirroring
   `SaveSystem.cs:748-782` (`m1_findings.md` §4.3).
6. Start the entry-immunity window keyed on `entityId`.
7. Reply `MIGRATE_IN_ACK`, or `MIGRATE_IN_NACK` on any failure.

**The absent-block rule, and it is the floor for every import** (added — §16, A32). A
`MIGRATE_IN` **without** a `species` block is valid — an old mod, an organism whose
`BibiteGenes.species` was null, or a block the sidecar's schema check stripped (§5.3) all
produce exactly the same frame, and the mod treats all three identically. The importing mod
**MUST remove `$.genes.speciesID` from the payload** before it restores. Removal, not a
substituted id: `BibiteGenes.LoadState` guards the lookup with `if (state["speciesID"] !=
null)` (`BibiteGenes.cs:581-583`), so an absent key skips the registry lookup entirely and
leaves `gene.species` null, and `ResumeBody` then classifies the arrival by the game's own
genetic distance through `CheckNewSpecies` (`BibiteBody.cs:477-480`). That is the game's
honest answer to "which local species is this organism", and it is available only when the
foreign integer is gone.

**No import path retains a raw foreign `speciesID`.** With a block, step 3(b) overwrites the
key with a local id; without one, this rule deletes it. There is no third branch, and a mod
that forwards the origin's integer into `LoadState` is defective — that integer is a
world-local counter value (`Species.speciesMaxID`, `Species.cs:19`) and the only thing it can
do in another world is collide.

**A block that arrives malformed is treated as absent.** A `species` object that breaks
§5.3's shape rules — a missing half, one parent field without the other, a non-string, a name
over 64 UTF-8 bytes — **MUST NOT** be partially applied. The mod ignores it whole, logs one
line naming it a sidecar defect (the sidecar was supposed to have stripped it, §5.3), and
takes the absent-block rule. Half a name is not a weaker identity; it is a different one.

**A failure to resolve or create is never a NACK.** If step 3(a) cannot produce a species —
the registry is unavailable, the create throws — the mod falls back to the absent-block rule,
logs one warning, and restores the organism anyway. Custody outranks bookkeeping: a name is
recoverable on the next hop, an organism refused at the door is not.

Step 6 is **REQUIRED under the ring, not optional** (amended — §14, A11). §4.3's inset
guarantees that **no arrival of any kind spawns inside a capture band** — every spawn
coordinate is at least `margin` inside the band's inner boundary `S − W`, on both axes,
including a bounce-back on its own export edge (amended — §15, A28). That guarantee buys
one tick, and one tick only. It is not what makes an immediate re-export impossible, and
the window is not redundant beside it.

**What the window is actually for** (amended — §15, A28). A **bounce-back** comes home
through the edge it left by, still carrying the velocity it left with, so it is moving
**outward** from a standing start `margin` inside the band — it re-enters that band under
its own power within a few `FixedUpdate`s, and a mod that spawns and exports across two
adjacent ticks makes one hop indistinguishable from two. **The grid adds a second case,
and the same window covers it** (added — §15, A19): the two axes are independent, so an
ordinary arrival that lands near a corner sits just inside the *other* axis's band and
crosses it the moment it travels that way. Neither case is a defect — a corner arrival that
swims north is a real north crossing — but neither may resolve in the arrival tick or in
the ticks immediately after it. The window is what separates the spawn from the export:
when it expires the organism is judged on where it actually is, like any other organism.
The mod **MUST** key the window on `entityId` and apply it to **both** bands.

A `null` return from `LoadBibiteOrEggFromData` is the normal failure signal — the method
swallows every exception (`m1_findings.md` §1.2). Reply `DESERIALIZE_FAILED`.

```json
{
  "protocol": "contract-a/2.1",
  "type": "MIGRATE_IN",
  "messageId": "7f2b91d6-0e34-4c7a-b158-9a03e6c2f411",
  "sentAt": 1785693600187,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "entityId": -843827577,
    "kind": "bibite",
    "gameVersion": "0.6.3.1",
    "payload": "{\"transform\":{\"position\":[2000.0,412.77],\"rotation\":274.11,\"scale\":0.9312},\"rb2d\":{\"px\":2000.0,\"py\":412.77,\"vx\":6.12,\"vy\":0.44,\"r\":274.11},\"genes\":{ ... },\"body\":{\"id\":-843827577, ... },\"clock\":{ ... },\"brain\":{ ... },\"version\":\"0.6.3.1\"}",
    "species": {
      "genericName": "Cyanea",
      "specificName": "velox",
      "parentGenericName": "Cyanea",
      "parentSpecificName": "prima"
    },
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

The same message off a **north** lane. The sender exported through `N` at
`exitPosition = 0.297`, so this world spawns it on its south edge at
`y = −S + W + margin` and `x = 2S · 0.297 − S = −812` (§4.3). Velocity and heading are
copied, so it arrives still travelling north (added — §15, A18):

```json
{
  "protocol": "contract-a/2.1",
  "type": "MIGRATE_IN",
  "messageId": "0a5e83b1-9c47-4d20-b6f8-31e70c9a5d42",
  "sentAt": 1785693612044,
  "data": {
    "migrationId": "7c2e5a90-4b18-4d63-9f07-2ab6c81d4e35",
    "entityId": -843827577,
    "kind": "bibite",
    "gameVersion": "0.6.3.1",
    "payload": "{\"transform\":{ ... },\"rb2d\":{ ... },\"genes\":{ ... },\"body\":{\"id\":-843827577, ... },\"clock\":{ ... },\"brain\":{ ... },\"version\":\"0.6.3.1\"}",
    "species": {
      "genericName": "Cyanea",
      "specificName": "velox",
      "parentGenericName": "Cyanea",
      "parentSpecificName": "prima"
    },
    "entryEdge": "S",
    "entryPosition": 0.297,
    "velocity": { "x": 61.2, "y": 88.9 },
    "heading": 34.55,
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
  "protocol": "contract-a/2.1",
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
(amended — §13, A7; `contract-b-m4.md` §9). It **MUST NOT** silently drop it — a drop is
the one failure mode D2 accepts, but it is never the first choice.

```json
{
  "protocol": "contract-a/2.1",
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
 |  CONFIG_UPDATE(reason=connect) ---> |   validate gameVersion, ringSlot, exportEdges
 |  <-------------------- EDGE_STATUS  |   epoch = 1, full state, one entry per export edge
 |  HEARTBEAT ------------------------>|   (every 1000 ms from here on)
 |  <----------------------- MIGRATE_IN|   replay of un-ACKed journal entries, paced
```

The mod does not migrate anything through **any** edge until it has applied an
`EDGE_STATUS` (amended — §15, A18).

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

The mod is stateless across world loads, and across socket reconnects it keeps exactly one
thing: the `migrationId` ledger below (amended — §15, A29). It still needs a key that lives
in the world data, not in mod memory, for everything a restart can lose.

| Key | Lifetime | Used for |
|---|---|---|
| `migrationId` | In memory, for the current **world session** — it survives a socket reconnect and clears only when `sessionId` changes (amended — §15, A29) | Fast O(1) rejection of any replay of a handled delivery — a lost `MIGRATE_IN_ACK`, or §7.5's un-ACKed replay after a reconnect — **even when the spawned organism has since died**, which is the case the entityId scan cannot catch and the case the slot-6 livelock was made of. |
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

### 7.5 Replay of un-ACKed inbound deliveries, and the delivery rate limit

After the handshake and the first `EDGE_STATUS`, the sidecar replays every journaled
inbound organism that has no `MIGRATE_IN_ACK`, in journal order, with `attempt`
incremented. Replay is unconditional — the sidecar does not try to guess whether the
previous delivery landed. The mod's `entityId` dedup absorbs the difference.

**The sidecar paces that release** (added — §15, A20). D15 ships a delivery rate limit
because the overnight run proved the failure it prevents: one slot slept for two hours, its
inbound deliveries and its west neighbour's contained export pile both accumulated, and both
released **together** at wake. The entry edge took hours of traffic in seconds
(`m4_considerations.md`, Question 9). A steady rate spreads a crowd across hours; a dam
delivers it in one breath, and the two need different answers.

| Rule | Statement |
|---|---|
| Where | **At the sidecar, never at the mod.** The journal is where a burst accumulates and the sidecar owns the journal. The mod keeps exactly one arrival path, so D8's "the mod learns nothing" is untouched. |
| The limit | At most `inboundRatePerSimMinute` (2.0) `MIGRATE_IN` frames released per **simulated** minute of the receiving world, with a token bucket of `inboundRateBurst` (5) so ordinary traffic is never delayed. |
| The clock | **Simulated**, not wall-clock. The sidecar advances it from `HEARTBEAT.simulatedTime` (§5.2), which it already receives every second. A world at 20× therefore drains a dam 20× faster in wall-clock terms and at the same rate *as the world experiences it*, which is the only frame of reference the crowd lives in. |
| A stopped clock | While `paused` is `true`, `timeScale` is `0`, or no `HEARTBEAT` has arrived for `pacingIdleGraceMs` (10 000 ms), `simulatedTime` does not advance and **nothing is released**. No tokens accrue in the dark, so a paused world does not bank a burst. |
| Order | Pacing **MUST NOT** reorder. Journal order is journal order, at any rate. |
| Scope | Every `MIGRATE_IN` the sidecar emits: fresh arrivals, replays after a reconnect, and bounce-backs. One queue, one rate. |
| Custody | Untouched. Pacing delays a delivery; it never drops one, never NACKs one, and never releases custody early. |
| A stalled mod | Delivery also holds while the mod's newest `HEARTBEAT` is older than `heartbeatDeliveryGraceMs` (1500 ms) — an intermediate state between keeping up and §8's `4004`, reached long before `pacingIdleGraceMs` stops the clock. A mod that is merely behind gets a quiet line, not a flood. It delays, never drops (added — §15, A29). |
| Replay backoff | A reconnect's un-ACKed replay batch waits `min((minAttempt − 1) · replayDelayStepMs, replayDelayCapMs)` before the pacer takes over — one delay for the whole batch, so journal order is preserved. A first-attempt batch (a sidecar-restart recovery, or a batch delivered once) is prompt; a batch that keeps forcing `4004` decays instead of re-flooding the instant the mod returns (added — §15, A29). |
| The bucket's lifetime | The token bucket resets only when `sessionId` changes. A same-session socket reconnect keeps it: the same world clock was running the whole time, the burst was spent, and a reconnect does not refund it (added — §15, A29). |
| Backpressure | A paced backlog is journal depth. When it passes `inboundQueueMax` the sidecar refuses **upstream** with Contract B `MIGRATION_NACK` / `OVERLOADED` (`contract-b-m4.md` §6.6, §6.8), which is a peer-local refusal and therefore proof that no custody moved — so the sender re-routes the organism to another slot instead of piling more onto a saturated one. Pacing and route-around compose into admission control for the whole map. |
| Observability | The depth waiting on the limit is a metric, not a wire field (WP3). A depth that never falls names a limit set too low. |

**Where the default comes from.** T1 measured slot 1 importing 6 003 organisms across 12.56
wall-clock hours at 20×, which is about **24 arrivals per simulated hour** — 0.4 per
simulated minute — into a population near 22. The default of 2.0 per simulated minute is
five times the measured natural rate, so no ordinary traffic is ever held back, and it caps a
two-hour outage's dam (about 900 organisms) at roughly 7.5 simulated hours of trickle rather
than one instant. Retune it from the entry-edge crowding metric, not from a guess.

> **The pacing interacts with the sender's hold timeout, and the answer is on the other
> wire** (Risk 9). Pacing delays `MIGRATE_IN`, which delays `MIGRATE_IN_ACK`, which delays
> the `MIGRATION_ACK` the sender is waiting for — so a deep backlog at a **live** peer looks
> like silence from the outside. `contract-b-m4.md` §9 answers it: the sender's hold clock
> runs **only while the destination is dark**, so a slow peer is never mistaken for an
> orphaned one. Do not answer it here by moving the custody gate: `MIGRATION_ACK` follows the
> receiving mod's `MIGRATE_IN_ACK`, and that gate is what makes the spawn the proof of
> delivery.

---

## 8. Heartbeat and liveness

| Direction | Mechanism | Interval | Timeout |
|---|---|---|---|
| mod → sidecar | `HEARTBEAT` message | `heartbeatIntervalMs` = 1000 ms wall clock | `heartbeatTimeoutMs` = 3500 ms |
| sidecar → mod | WebSocket ping / pong frames | `wsPingIntervalMs` = 15000 ms | `wsPongTimeoutMs` = 10000 ms |

The mod's heartbeat timer runs on wall-clock time, not simulated time. A paused sim, a
`0×` time scale and a 20× time scale all produce the same heartbeat cadence.

**Before heartbeats are stopped, they go quiet, and the sidecar reacts to quiet first**
(added — §15, A29): once the newest `HEARTBEAT` is older than `heartbeatDeliveryGraceMs`
(1500 ms), the sidecar holds further `MIGRATE_IN` release (§7.5) while the countdown to
`heartbeatTimeoutMs` runs. A mod that recovers inside the window resumes delivery and
nothing else happens; a mod that does not still gets the `4004` below, on schedule. What
this removes is the third outcome the slot-6 livelock exposed: a sidecar force-feeding
deliveries into a stalling main thread for the whole 3.5 s and then replaying all of them.

**When heartbeats stop**, the sidecar **MUST**, in this order:

1. Close the WebSocket with `4004`.
2. Mark **every** local export edge closed, and publish `modConnected: false` over
   Contract B. The relay then drops this peer out of every other peer's **deliverable** set,
   so their lanes re-pair around it instead of closing (`contract-b-m4.md` §8). A dead sim
   must not keep receiving organisms — and under M4 it must also not stop the current
   (amended — §15, A18).
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
| `NO_ROUTE` | transient | The edge is open but the sidecar has no destination for it: the relay has not granted a slot yet, or the effective-neighbour walk on that axis found no deliverable slot between the last `EDGE_STATUS` and this frame (amended — §15, A18). | Revive. Retry after `retryAfterMs`. |
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

**M4 is the multi-edge sim that debt was waiting for, and it still does not need the field**
(amended — §15, A22). The correlation was never about how many edges exist: the mod binds one
`migrationId` to one organism and one `exitEdge` (§5.3, §6.3), so a NACK naming that
`migrationId` names that edge. What M4 *does* add is an obligation, and it is what keeps the
debt moot: **the mod MUST record `exitEdge` in its in-flight record**, and on a
`transient` NACK it closes **only that edge** until the next `EDGE_STATUS`, never both. A mod
that closes every edge on one `EDGE_CLOSED` throws away a lane that is open.

**M4 adds no `MIGRATE_OUT_NACK` code and no `MIGRATE_IN_NACK` code.** Every M4 failure mode
on this wire is an existing code applied per edge: a closed lane is `EDGE_CLOSED`, an
unrouteable map is `NO_ROUTE`, an `exitEdge` outside the declared `exportEdges` is
`MALFORMED_MESSAGE`. The new failure modes M4 does introduce — a vacant slot, an offline
peer, a frame the relay never forwarded — live entirely on Contract B, because they are
statements about the map and the mod has none (`contract-b-m4.md` §6.8).

**No code in this table is ever caused by `parents`** (added — §14, A12). The lineage annex
is best-effort by construction: a missing, oversized, malformed or unhashable parent blob
is recorded as a gap and the migration proceeds. `INVALID_PAYLOAD` refers to the migrant's
own `payload` and to nothing else.

**Nor by `species`** (added — §16, A30). The block is optional, and a block that fails its
shape rules is **stripped** and forwarded without — logged once, never NACKed (§5.3). This is
a deliberate departure from §9.3's default answer for a `data` field that fails validation,
and the reason is the same asymmetry the annex rests on: refusing an organism over a label
trades a live migrant for a cosmetic, and the label is recoverable on the organism's next hop
while the organism is not.

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

**No code in this table is ever caused by the `species` block** (added — §16, A32). An absent
block is valid and takes the absent-block rule; a resolve or create that fails falls back to
the same rule and the organism still spawns (§5.7). The one way species handling can reach
this table is a **defective rewrite**: a step 3(b) that produces invalid JSON breaks the
restore and surfaces as `DESERIALIZE_FAILED`, exactly as a botched position rewrite would, and
it is a mod defect either way.

### 9.3 What is a NACK and what is a close

| Situation | Answer |
|---|---|
| Bad JSON, missing envelope field, wrong envelope type | Close `4003` |
| Unsupported `protocol` major version | Close `4000` |
| Unknown `type` | Ignore, log one warning, keep the connection |
| Unknown field inside `data` | Ignore silently |
| A `data` field fails validation on a type that **has** a NACK (`MIGRATE_OUT`, `MIGRATE_IN`) | The matching NACK with `MALFORMED_MESSAGE`, keep the connection. **One named exception:** a malformed `species` block is stripped and the frame proceeds without it (amended — §16, A30) |
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
| `port` | `8787` | both | The sidecar's listen port and the mod's connect port. The M3 rig used `8787`–`8789` for slots 1 to 3; the M4 rig runs six instances, so it uses `8787` to `8792` for slots 1 to 6 (amended — §15, A18). |
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
| `entryImmunitySeconds` | `5` | mod | Simulated seconds after arrival during which an organism cannot re-trigger the border strip (`m2_findings.md` §4.4). Applies to **both** capture bands (§15, A19). |
| `entryMargin` | `max(5, 0.5·W)` | mod | World units the spawn is inset **past** the strip's inner face, on top of `W` (§4.3). Named here because §4.3's entry formula now covers all four edges (added — §15, A19). |
| `inboundRatePerSimMinute` | `2.0` | sidecar | Maximum `MIGRATE_IN` deliveries released per **simulated** minute of the receiving world (§7.5, §15 A20). |
| `inboundRateBurst` | `5` | sidecar | Token-bucket capacity for that rate, so ordinary traffic is never delayed (§7.5, §15 A20). |
| `pacingIdleGraceMs` | `10000` | sidecar | Wall-clock silence from `HEARTBEAT` after which the pacing clock stops advancing (§7.5, §15 A20). |
| `heartbeatDeliveryGraceMs` | `1500` | sidecar | Age of the newest `HEARTBEAT` beyond which `MIGRATE_IN` release is held — the quiet-mod gate that trips before `heartbeatTimeoutMs` does (§7.5, §8, §15 A29). |
| `replayDelayStepMs` | `500` | sidecar | Per-generation step of the reconnect replay delay, keyed on the batch's least-delivered `attempt` (§7.5, §15 A29). |
| `replayDelayCapMs` | `5000` | sidecar | Ceiling of that delay (§7.5, §15 A29). |

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
- **Pace from the mod's simulated clock, not from yours** (added — §15, A20). Keep the last
  `HEARTBEAT.simulatedTime` and the last arrival instant; the token bucket refills by
  `(simulatedTimeNow − simulatedTimeThen) / 60 · inboundRatePerSimMinute`. A heartbeat that
  goes backwards is a world that reloaded an older save — clamp the delta at zero, do not
  refund tokens, and do not treat it as an error. A heartbeat that stops means the world
  stopped: freeze the bucket, do not accrue against wall time.
- **Pacing is a release gate, not a queue.** Keep the journal as the queue and gate the
  *send*. A second in-memory queue in front of the socket duplicates the ordering problem and
  loses the paced entries on a restart.

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
- **Parse the payload with `Newtonsoft.Json.Linq` only for the eight position numbers and
  `$.genes.speciesID`** (amended — §16, A31). Use `JObject.Parse` / `ToString()`. Do not
  deserialize it into a typed model — that would re-introduce the C# schema D4 removed. The
  species key is a **write or a removal**, never a read: the incoming value is a foreign
  world's counter and there is nothing in it to learn.
- **Do the species resolve before `LoadBibiteOrEggFromData`, on the main thread, and cache the
  result per name for the tick** (added — §16, A31). `GlobalLineageManager.recordedSpecies` is
  a `List<Species>` and the match is a linear scan, so a burst of arrivals from one species
  would otherwise rescan the registry once per organism inside one `FixedUpdate` — the same
  shape as the sibling-serialization cost §5.3 caches away. The cache is per tick: a create in
  this tick must be visible to the next arrival in it, and nothing may hold a `Species`
  reference across ticks.
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
- **Keep the two export edges in one state object, and read it once per tick**
  (added — §15, A18). The corner rule (§4.3.2) needs both `open` flags in the same
  `FixedUpdate`, and `EDGE_STATUS` is applied in `Update`; copy the applied state into an
  immutable snapshot the tick reads, so a frame that lands mid-tick cannot change the answer
  half way through the organism loop.
- **Test the bands in the order `E` then `N`, and let §4.3.2 break the tie** — do not `break`
  out of the loop on the first band that matches. A first-match loop silently implements
  "whichever edge I wrote first wins", which is the exact defect the corner rule exists to
  prevent, and it will not show up until an organism reaches a corner at speed.
- **One in-flight record per organism, and it holds the edge** (§9.1, §15 A22). The record
  is what turns an edgeless `MIGRATE_OUT_NACK` into "close the north lane", and closing both
  lanes because one NACK arrived is a real and easy bug.

---

## 12. Open items for the owner

1. **No authentication in M2, M3 or M4** (amended — §14, A17; §15, A24). Any local
   process can connect to the sidecar and drive migrations, and any local process can
   impersonate the sidecar to the mod. Contract A binds loopback only, and D9 keeps the mod
   and its sidecar on one machine, so neither the LAN milestone nor the operations milestone
   changes this contract's exposure. The shared token M3 adds lives on **Contract B**, where
   the wire leaves the loopback (`contract-b-m4.md` §3). A bearer token here waits for **M5**,
   with TLS and the rest of the public-exposure set — D16 renumbered that milestone, and the
   exposure it brings is unchanged.
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
   would need if it ever returned. **M4 is that multi-edge sim and the field is still not
   needed** (§15, A22): the correlation is on `migrationId` and the mod's own in-flight
   record holds the edge. The item stays open because the *reason* it is moot changed — from
   "there is only one edge" to "the mod records the edge" — and that second reason is an
   obligation somebody could drop.
7. **The capture band's direction test has no magnitude floor** (§4.3.1). `velocity.x > 0`
   admits an export on a single tick of eastward jitter, so an organism loitering in the
   band can cross without ever meaning to. M3 accepts it: a floor is a second tunable, it
   must scale with `S`, and the one-way ring bounds the consequence to "the organism moved
   on". Open because it is a **rate** question that only a populated ring can answer, and
   the answer changes a rule the mod enforces on every organism on every tick.
8. **A parent blob dropped for frame size is unreachable at the sidecar** (§14, A12). The
   sidecar records it as `"parent_gone"` because the two look identical on the wire, so
   `contract-b-m4.md` §6.6's `"blob_dropped_for_size"` is defined and never emitted. The
   fix is one additive optional field on `parents[]`; M3 does not add it.
9. **Neither side refuses a non-grid `exportEdges` set at `CONFIG_UPDATE`** (§5.1, §15 A18,
   §15 A26). A18 requires at least one member, no duplicates, and every member also in
   `borderEdges`, and both implementations enforce exactly that. A set the map cannot use —
   `["E", "S"]`, say — passes every check, handshakes cleanly, and is then refused **one
   organism at a time** at `MIGRATE_OUT`, because M4's grid exports east and north and
   nothing else. The mod additionally rejects an edge declared with its opposite, which
   catches `["E", "W"]` but not `["E", "S"]`. Open rather than fixed because the rule that
   would close it is a **topology** rule — "these are the map's export axes" — and A18's whole
   point is that the mod declares geometry and learns no topology. The honest fix is a
   startup refusal in the sidecar, which knows the map; M4 does not add it, and a
   misconfigured slot is therefore diagnosed from a stream of NACKs rather than from one
   startup error.

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
| A Contract B `MIGRATION_NACK` arrived, any code | **Bounce.** `contract-b-m4.md` §6.8 forbids a NACK after durable custody, so a NACK proves custody never moved. |
| The forward never reached a live peer — relay link down, or the destination ring slot vacant — for longer than `bounceTimeoutMs` | **Bounce.** The frame was never handed to anyone. |
| The forward reached a live peer and no answer came back | **Hold and re-forward.** Never bounce. The destination deduplicates on `migrationId`, so a re-forward costs nothing and holding turns a possible duplication into a bounded delay. |

**Enforced by:** the sidecar. Already specified in `contract-b-m4.md` §9 (and in the
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
Contract B in M2 or M3. `contract-b-m4.md` §9 and §13 item 2 already say this; §5.9 and the
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
**and M4** unchanged; the current text for each is in `contracts/contract-b-m4.md`, which
supersedes `contract-b-m3.md` (amended — §15, A18).

| Resolution | Where it is written | Summary |
|---|---|---|
| `entityId` and `heading` travel explicitly on the Contract B wire | `contract-b-m4.md` §6.6, §11 item 3 | Additive fields. `entityId` from the blob wins over the wire value, with a warning on a mismatch, because the blob is what the destination restores. `heading` is the reverse: the wire value wins, because the receiving mod rewrites the blob from it (§5.7 step 3). |
| The shape of `$.body.id` in a bb8 blob | `contract-b-m4.md` §11 item 3 | Both `{"body":{"id":N}}` and `{"body":{"id":{"id":N}}}` are accepted. §5.3's example elides the `BibiteID` wrapper, so the two are indistinguishable from this document alone. |
| The sidecar persists what the relay granted it | `contract-b-m4.md` §7.4 | `<data-dir>/slot` and `<data-dir>/peer-id`, replayed as `preferredSlot` and `peerId` on the next claim, so a relay restart cannot swap two sims and a sidecar restart cannot lose its slot. In M2 this was `<data-dir>/sector`. |

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

*Work order item 6 (D8).* **Generalized by §15, A18: one export edge became a declared set,
and `EDGE_STATUS` now carries one entry per member. Everything below about the passive entry
edge, the reason mapping and the immunity window survives unchanged — read `exportEdge` as
"each member of `exportEdges`" throughout.**

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
| Entry immunity | REQUIRED, not optional. Geometry keeps an ordinary arrival away from the band it would export through; only the immunity window stops a bounce-back, which lands inset but still moving outward, from re-entering its own band and exporting again (amended — §15, A28). |

The ripple is one-way, like the lanes: when a peer dies, its **west** neighbour loses its
export target and closes its export edge; its east neighbour simply receives nothing and is
told nothing (`contract-b-m3.md` §8).

> **Superseded in its second half by D12** (amended — §15, A18). The ripple keeps its
> direction — a dark peer is still only ever announced *backwards* along a lane — but the
> west neighbour no longer **closes**: it re-pairs to the next deliverable slot on that axis
> and keeps exporting. An edge closes only when the whole axis holds no deliverable slot
> (`contract-b-m4.md` §8). Everything else in this amendment stands.

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
`contract-b-m4.md` defines three `gapReason` values, and the sidecar can only ever emit two
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

*Work order item 9 (D8).* **Re-assessed by §15, A22 under two export edges: still open, still
moot, but for a different reason — the mod's in-flight record, not the count of edges. The
word "permanently" below was written about a topology that M4 replaced.**

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

*Work order item 10, reconciled (D8).* **The `<major>.<minor>` scheme and the major-only
compatibility rule this amendment introduced are unchanged. M4 is the case it was built to
handle honestly: §15, A23 applies the same test to `exportEdges`, gets the opposite answer,
and takes the major bump to `contract-a/2.0`.**

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
obvious M3 addition". D9 then defined M3 as a LAN milestone and moved public exposure to the
next milestone, which changes the answer. That milestone was numbered M4 when this amendment
was written and is **M5** after D16's renumbering (amended — §15, A24).

**Resolution.** Contract A stays on `127.0.0.1` with no authentication in M3. The mod and
its sidecar always share a machine (D9), so this wire never leaves the loopback and the M3
threat model does not reach it. The token M3 does add belongs to **Contract B**, where the
wire crosses the LAN (`contract-b-m4.md` §3). A bearer token here is **M5** work, alongside
TLS and the rest of the public-exposure set (amended — §15, A24). **M4 confirmed the same
answer for the same reason:** it is still a LAN milestone, the mod and its sidecar still share
a machine, and this wire still never leaves the loopback.

The residual risk is unchanged and accepted: any local process can drive migrations or
impersonate the sidecar. On a single-owner machine that is the same trust boundary as the
game's own save directory.

**Enforced by:** neither, deliberately. Recorded so that the next reader of §12 item 1 does
not implement a token that no milestone asked for.

---

## 15. M4 amendments (`contract-a/2.0`, 2026-08-05)

Decisions D12–D16 were ratified on 2026-08-05 and redefined M4, and the owner signed the
design off the same day (`system_decomposition.md`; `m4_considerations.md`, *Owner
Sign-Offs*). **Twelve amendments, A18 to A29, in four batches.** The first eight were written from
the design and carry the four items that document's *Contract Changes Needed* table assigns to
Contract A — items 9, 10, 11 and 15 — plus the four consequences that fall out of them: the
major version bump (item 12), the milestone renumbering (item 13), one optional observability
field that the exit test needs and the table did not name, and an explicit list of the things
M4 does **not** change. **The last two, A26 and A27, were written from the code** — see below.

The section follows §13's and §14's pattern. Each amendment names the ambiguity or change,
the resolution, and **which side enforces it** — the side whose code makes the rule true, and
which therefore has to change if the rule changes. Where an amendment contradicts the body,
§13 or §14, the amendment wins, and the body carries an `(amended — §15, Ax)` or
`(added — §15, Ax)` marker at each such point.

Unlike §13 and §14, this set is **breaking**. A18 removes a field and replaces it with one of
a different type, which §3.1's own rule answers with a major bump — see A23.

**A26 and A27 were folded in on 2026-08-05, after the mod implementation landed** (commit
`efd74a1`), by the reconciliation pass that read the code back against this document. Both are
**clarifying, not breaking**: they add no field, remove none, and change no enum, so
`contract-a/2.0` is unchanged and no implementation has to move. A26 generalizes a rule the
mod already implements more generally than §4.3.2 stated it; A27 writes down a behaviour this
document left entirely unspecified, which is the more dangerous of the two gaps because
nothing in it was wrong — there was simply nothing there. `contract-b-m4.md` §14 carries the
matching set for the other wire, B4 to B7, and none of the seven interacts with another.

**A28 was folded in on 2026-08-05 by the M4 pre-flight pass**, and it is the third kind of
gap again: not a missing rule and not a narrow one, but **a stated geometry that the mod does
not produce**. `BorderGeometry.EntryPoint` clamps the free coordinate as well as insetting the
fixed one, which §4.3 never said — and three sentences across both contracts had reasoned from
the unclamped formula to conclusions that are arithmetically unreachable. A28 states the
clamp, states the guarantee it buys, and re-derives the entry-immunity window's justification
from the ticks *after* the spawn rather than from the spawn itself. Clarifying, like A26 and
A27: no field, no enum, no default, no implementation change.

**A29 was folded in on 2026-08-06, from the slot-6 livelock**, and it is a fourth kind of
gap: no single rule was missing or wrong, but four rules that are each correct composed into
a failure none of them names. A morning arrival flood, a 64-deep paced backlog, one long
`FixedUpdate`, the mandatory `4004`, and an unconditional replay closed into a cycle that
held a live world at ~1 TPS for hours. A29 records the defences both sides now carry —
delivery that pauses for a quiet mod, a session-scoped pacing bucket, an escalating replay
delay, a mod-side ingest budget, and a dedup ledger that survives a reconnect. **The
message catalogue, every field and every enum are untouched**; three named defaults are
added to §10, so `contract-a/2.0` is unchanged and the wire needs no version bump.
`contract-b-m4.md` §14 B8 carries the matching pair for the other wire.

> ### Why an amendment set and not a successor document
>
> Contract B took the opposite decision for the same milestone: `contract-b-m4.md` is a new
> file that supersedes `contract-b-m3.md` in full. The two answers are not inconsistent —
> they come from applying the same three tests, which `contract-b-m3.md`'s own box states, to
> two documents in very different shapes:
>
> 1. **Is the file milestone-named and milestone-scoped?** Contract B's was: it is titled
>    *M3* and its §1 opens "In scope for M3: one ring of three slots". This file is titled
>    *Contract A — Mod ↔ Sidecar Wire Specification*, and its scope is the process boundary,
>    not a milestone. Nothing in its filename, title or first paragraph becomes false.
> 2. **Is the change structural?** Contract B's is: the ring becomes a grid, the claim and
>    grant messages change shape, insertion is rewritten, a relay answer is new, and half the
>    catalogue's semantics move. Contract A's is not. **The message catalogue is untouched** —
>    the same nine types, the same directions, the same answers, the same custody chain. Two
>    message bodies change one field each, one geometry section grows a second band, and one
>    delivery rule gains a rate limit.
> 3. **Is the old text still needed as it stands?** Contract B's is, as the specification the
>    passing M3 exit test ran against. Contract A's body is still current in every section it
>    describes; a successor file would be a copy with four edits, and the amendment-set
>    pattern is what has kept those edits legible twice already.
>
> **A major version bump is not by itself a reason to split the file.** The version identifies
> the *wire*, and the file identifies the *interface*. This document has always carried its
> own history in §13 and §14, each with its own protocol identifier, and a reader who needs to
> know what `contract-a/1.1` said reads §14 beside the marked body text. What a major bump
> does change is the URL path, and A23 states that.

### A18 — `exportEdges` replaces `exportEdge`, and `EDGE_STATUS` carries one entry per export edge (§1, §2, §4.2, §4.4, §5.1, §5.3, §5.4, §5.7, §8, §9.1)

*Work order item 9 (D13). **This is the breaking change**, and it is the reason M4 exists
before M5 rather than after it.*

**Change.** D8 gave every sim exactly one export edge and one passive entry edge. D13
generalizes the map to two axes: a peer **exports east and north** and **receives west and
south**, so a singular `exportEdge` and a single-entry `EDGE_STATUS` cannot express the
world any more.

**Resolution.**

| Rule | Statement |
|---|---|
| `CONFIG_UPDATE.exportEdges` | An array of edge enums, REQUIRED, at least one member, no duplicates, every member also in `borderEdges`. `["E", "N"]` under the grid. It declares **geometry** — "I run a capture band on these edges" — never topology. |
| `CONFIG_UPDATE.exportEdge` | **Removed.** No fallback: an M3 mod cannot reach the field-validation stage, because its `protocol` major is rejected first (A23). |
| `CONFIG_UPDATE.borderEdges` | Now all four edges, `["E", "N", "W", "S"]`. It keeps the meaning §14 A11 gave it: the edges on which this mod will **accept** an inbound organism. |
| `EDGE_STATUS.edges` | **One entry per declared export edge**, each with its own `open`, `reason` and `peerSimulationSize`. Full state, one `epoch` for the frame. A declared edge with no entry is **closed**. An entry for an undeclared edge is ignored with a warning, never a close. |
| `MIGRATE_OUT.exitEdge` | `"E"` or `"N"`, always a member of the declared `exportEdges`, always the edge §4.3.2 chose. |
| `MIGRATE_IN.entryEdge` | The **opposite** of the sender's `exitEdge` for ordinary traffic — `"W"` off an east lane, `"S"` off a north lane — and the origin's own exit edge for a bounce-back. All four values now appear. |
| Independence | The two edges open and close independently. A peer whose whole row went dark still exports north; a peer alone in the map closes both with `no_peer`. |

**The mod still learns no topology, and that is the property this amendment protects.**
`exportEdges` names edges of the mod's own square. It never names a neighbour, a slot, a
coordinate, a skipped slot or a map shape. Route-around, the effective-neighbour walk, the
grid and the healing all happen behind `EDGE_STATUS`, exactly as the ring did
(`contract-b-m4.md` §8). A mod that could tell a healed lane from a healthy one would be a
mod with topology in it.

**Why this had to be a wire change and not a mod-side one.** A mod could have been given two
edges while the wire kept a singular field, by opening a second connection or by treating the
north lane as a special case. Both put topology in the mod. The array is the only shape where
the mod says what it *is* and the sidecar says what is *reachable*, which is the split §1 has
had since M2.

**Enforced by:** the mod for declaring the set, for one export per organism, and for
applying an `EDGE_STATUS` frame atomically; the sidecar for producing one entry per declared
edge, for never opening an undeclared edge, and for carrying the set to the relay
(`contract-b-m4.md` §6.3).

### A19 — A second capture band, the corner rule, and an entry point on every edge (§4.3, §4.3.1, §4.3.2, §5.3, §5.7, §10)

*Work order item 10 (D13).*

**Change.** One band on the east edge becomes one band per export edge, and the two bands
overlap in the north-east corner. The entry side gains a second passive edge on the south,
and a bounce-back can now come home through either export edge — so the entry-point formula,
which §4.3 gave only for `W`, needs all four cases.

**Resolution.**

1. **A band per export edge**, with the same rule on each axis: `x ≥ S − W ∧ vx > 0` on `E`,
   `y ≥ S − W ∧ vy > 0` on `N`. Half-open, outward-unbounded, tested every `FixedUpdate`.
   Every clause of §14 A13 carries over verbatim to the second band, including the reason it
   reaches outside the square and the reason the direction test is what separates an export
   from a wrap.
2. **The corner rule** (§4.3.2): candidates are the bands the organism is in **intersected
   with the edges that are open**; one candidate wins outright; two are decided by
   `velocity.x ≥ velocity.y → "E", else "N"`. Exactly one `MIGRATE_OUT` per organism.
3. **Openness filters before the tie-break.** An organism in both bands with only one open
   lane exports through the open one. This is not an optimization: without it a corner is a
   trap whenever one axis is dark, and the T1 run measured what a trap at an edge costs.
4. **The entry point on all four edges** (§4.3): inset the fixed coordinate by `W + margin`
   inward, take the free coordinate from §4.3's inverse table, and clamp that free
   coordinate by the same inset (amended — §15, A28). `entryMargin` is named in §10 for the
   first time, because the formula now has four callers instead of one.
5. **Entry immunity covers both bands**, keyed on `entityId`. It was REQUIRED for the
   bounce-back case (§14, A11) and it is now REQUIRED for a second, ordinary case: an arrival
   through `W` near a corner lands just inside the north band's inner boundary and crosses it
   the moment it travels north, and an export in the arrival tick — or in the two after it —
   would make one hop look like two (amended — §15, A28: the clamp means the arrival lands
   *near* that boundary, never *past* it).

**Why the larger outward component, and why no epsilon.** The rule has to be a *function* —
two mods reading the same organism must produce the same edge, or the same world exports the
same organism two different ways depending on which build is running. Velocity is the
organism's intent and position is an accident of the tick, so velocity decides; `≥` folds the
tie into the `E` branch so there is no third case to get wrong; and an epsilon would only
enlarge the region where a third rule would be needed. §4.1's ban on float equality is about
comparing two independently-derived values, and this is one `Rigidbody2D` read compared with
itself.

**Enforced by:** the mod, entirely. The sidecar never sees a position, never converts one
(§4.3), and cannot check a corner decision. This is the amendment with the least external
verification in the whole contract, which is why the exit test counts organisms by entity ID
and asserts that no organism exports twice.

### A20 — The sidecar paces inbound delivery, in simulated time (§5.7, §7.5, §10)

*Work order item 15 (D15). The owner named the mechanism and signed the mitigation off on
2026-08-05.*

**Change.** T1 left a mass of organisms on one world's west border. The cause is not a rate,
it is a dam: a slot slept for two hours, its inbound deliveries queued in its own journal
while its west neighbour's export pile queued in that neighbour's, and both released together
at wake. M4 therefore paces delivery out of the journal.

**Resolution.** §7.5 carries the full rule. In summary:

- The sidecar releases at most `inboundRatePerSimMinute` (2.0) `MIGRATE_IN` frames per
  **simulated** minute of the receiving world, through a token bucket of `inboundRateBurst`
  (5) so ordinary traffic never waits.
- The clock is the mod's own `HEARTBEAT.simulatedTime`. It does not advance while the world
  is paused, stopped or silent, and it never runs on wall time.
- Journal order is preserved absolutely. Bounce-backs and replays go through the same gate.
- Custody is untouched: pacing changes when an organism arrives, never whether it arrives.
- A backlog that passes `inboundQueueMax` is refused **upstream** with Contract B
  `OVERLOADED`, which is a peer-local refusal and therefore proof no custody moved — so the
  sender re-routes instead of queueing more.

**No field is added to this wire, and that is deliberate.** The mod keeps one arrival path,
learns nothing new, and needs no change (D8, D15). The observability the operator needs —
journal depth waiting on the limit — is a sidecar metric and a status-page value
(`m4_considerations.md`, WP3, WP5), not a message field: putting a queue depth in
`EDGE_STATUS` would make an export-edge message carry an inbound-queue fact, and would hand
the mod a number it has no rule for.

**Enforced by:** the sidecar. The mod's only obligation is the one it already had — answer
every `MIGRATE_IN` — plus the negative obligation of A25: never infer anything from arrival
cadence.

### A21 — `HEARTBEAT` carries an optional save receipt (§5.2, §10)

*A consequence of D14 and D15 that the work order's contract table does not name. Recorded
here as a deliberate addition, with its justification, so the next reader can reverse it.*

**Ambiguity.** D14 gives the mod a periodic world save; D15's status page must show "the last
save of each world" (`m4_considerations.md`, Exit Test Part 9). The archive serves that page
from the relay's ring view, and durable save metrics are files on the machine that wrote
them. For the two instances on the **second computer** there is no path from a local metrics
file to the archive, and D9 keeps it that way — the rig never drives the far machine.

**Resolution.** `HEARTBEAT` gains an OPTIONAL `lastSave` object: `atMs`, `simulatedTime`,
`population`, and the optional `name`, `bytes` and `durationMs`. The sidecar carries it to
the relay in its peer stats (`contract-b-m4.md` §6.3.1), and the relay republishes it in
`PEER_STATUS`, where the archive already reads.

Three rules keep it honest:

- **It is a receipt, not a request.** No component asks for a save, schedules one, or reacts
  to a missing one. The mod owns the save timer (D14).
- **It is informational** (D5). `atMs` is a label on another machine's clock, and no
  correctness decision reads it.
- **OPTIONAL means optional.** A mod that omits it is conformant, and the status page shows
  the world's save state as **unknown** — which Risk 4 requires it to do honestly anyway.

`durationMs` is the field that proves the owner's 2-second save budget at six instances,
which is otherwise measurable only by watching a log on a machine nobody is watching.

**Enforced by:** the mod, for emitting it truthfully or not at all. The sidecar copies it and
never computes it.

### A22 — Contract debt A5 survives the second export edge, still open, still moot (§9.1, §12 item 6, §13 A5, §14 A15)

*Work order item 11 (D13).*

**Assessment.** §13 A5 accepted that `MIGRATE_OUT_NACK` carries no `edge`, on the grounds
that M2 declared one edge. §14 A15 re-assessed it under the ring and called the condition
permanent. M4 declares two edges, so the stated grounds have expired — and the debt is still
moot, for a reason that was always the real one.

**Resolution.** The correlation was never a count of edges. The mod binds one `migrationId`
to one organism and one `exitEdge` (§5.3, §6.3), and a NACK names the `migrationId`. What
changes with a second edge is the **obligation** that makes that true, and M4 states it as a
requirement rather than an implication: **the mod MUST record `exitEdge` in its in-flight
record, and MUST close only that edge on a transient NACK.**

`MIGRATE_OUT_NACK` therefore gains nothing, and §12 item 6 stays open with its reasoning
updated. Adding the field now would be additive and harmless — and it would also be the
second way to answer a question that already has an answer, which is how two implementations
start disagreeing about which one is authoritative.

**Enforced by:** the mod, which is a change from §13 A5's "neither, today". The debt is no
longer inert: it rests on a rule somebody could drop, and dropping it costs a live lane every
time the other one closes.

### A23 — `contract-a/2.0` is a major bump, and the path moves to `/contract-a/v2` (§2, §3, §3.1)

*Work order item 12 (D13).*

**Change.** §3.1's rule is explicit: additive fields raise the **minor**; field removal, type
changes and enum-value removal require a **major**. A18 removes `exportEdge` and replaces it
with `exportEdges`, an array. That is both a removal and a type change.

**Resolution.** Apply the contract's own test honestly, item by item:

| M4 change to Contract A | Kind | Needs a major? |
|---|---|---|
| `CONFIG_UPDATE.exportEdge` removed | field removal | **yes** |
| `CONFIG_UPDATE.exportEdges` added, an array where a string was | type change at the same concept | **yes** |
| `CONFIG_UPDATE.borderEdges` widened to four values | same field, same type, wider value set | no |
| `EDGE_STATUS.edges` widened from exactly one entry to one per export edge | a constraint relaxed on an unchanged field | no |
| `HEARTBEAT.lastSave` added | additive optional field | no |
| `entryMargin`, pacing tunables named in §10 | defaults, not wire | no |
| second capture band, corner rule, entry immunity on both bands | mod-side behaviour, no wire shape | no |

Two rows are enough. The identifier is **`contract-a/2.0`**, and by §3.1 the URL path moves
with the major to **`/contract-a/v2`**.

**The migration note for the M3 rig.** A `contract-a/1.1` mod and a `contract-a/2.0` sidecar
are incompatible **by design**, and both sides say so loudly rather than misreading a field:

1. The M3 mod dials `/contract-a/v1`. The M4 sidecar **MUST** keep serving that path and
   **MUST** close every connection on it immediately with `4000 PROTOCOL_UNSUPPORTED` (§2).
   A bare HTTP 404 would be a socket error in a BepInEx log and half an evening of diagnosis;
   `4000` is a defined close that the mod already handles by logging one loud error and not
   reconnecting (§2.1, §6.2).
2. An M3 mod that somehow reaches `/contract-a/v2` is closed with `4000` at the envelope
   check, before any field is read.
3. There is no field-level fallback anywhere. A18 could have kept `exportEdge` as a
   deprecated alias, and did not: a compatibility path that only an already-rejected peer can
   take is dead code that reads like a supported configuration.
4. **The rig upgrades in one step**, and this is the milestone in which that is cheap: three
   binaries and one mod, all built by the owner, none in a stranger's hands. After M5 the same
   change would cost a migration story for people the owner cannot reach — which is the whole
   argument for landing it now (D13).

**Enforced by:** both, symmetrically. Each side sends `"contract-a/2.0"` and compares only
the major. The sidecar additionally owns the retired path and its `4000`.
**The version string moved on with §16, A33** — each side now sends `"contract-a/2.1"` — and
everything else in this amendment stands, including the path and the `4000` on `v1`: a minor
bump moves no path (amended — §16, A33).

### A24 — D16's renumbering: the "M4" that meant public release is M5 (§4.5, §12 item 1, §14 A17)

*Work order item 13 (D16).*

**Change.** D9 moved public exposure out of M3 into what was then M4. D16 then inserted the
operations milestone: M4 is operations, **public release is M5**, direct P2P is M6, and
ecosystem completeness with the corpse, pellet and egg payloads is **M7**. Every milestone
number this document wrote before 2026-08-05 for a future milestone is one too low.

**Resolution.** Corrected in place, at each of the four points, with no change of meaning:

| Where | Said | Says |
|---|---|---|
| §4.5 | `"corpse"`, `"pellet"`, `"egg"` reserved for M6 | reserved for **M7** |
| §4.2 | M6's payload kinds may not share the ring's geometry | **M7's** |
| §12 item 1 | a bearer token here waits for M4 | waits for **M5** |
| §14 A17 | D9 moved public exposure to M4 | to the milestone now numbered **M5** |

Nothing else moves: M2 and M3 are history and keep their numbers, and every `(amended — §13)`
and `(amended — §14)` marker refers to a milestone that happened.

**Enforced by:** neither. It is documentation hygiene, and it exists so an implementer does
not read "waits for M4" in the milestone that *is* M4 and build a bearer token nobody asked
for.

### A25 — What M4 deliberately does not change (§1, §4.3, §4.4, §5.1, §5.7, §6.3, §12)

*Recorded because M4 is a large milestone next door, and the cheapest defect to introduce
here is one that was never asked for.*

| Unchanged | Why it is worth stating |
|---|---|
| **The message catalogue.** Nine types, same directions, same answers. | M4 adds no message to this wire. Route-around, healing, insertion, handover, the non-delivery proof and the bounded hold are **all** Contract B (`contract-b-m4.md` §6.8, §7, §9). |
| **The custody chain.** `MIGRATE_OUT_ACK` after the durable journal write; `MIGRATION_ACK` after the receiving mod's `MIGRATE_IN_ACK`. | D2's one bounded exception is a *sender-side hold* on the other wire. Nothing about the moment custody moves has changed, and no pacing, hold or re-route may move that gate. |
| **A bounce-back is an ordinary `MIGRATE_IN` with `bounceBack: true`.** | An automatic bounce at the hold timeout arrives at the mod exactly like a bounce after a NACK. The mod has no timeout concept and needs none. |
| **The mod learns no topology.** No coordinate, no slot position, no neighbour, no skipped slot, no map shape. `ringSlot` stays a configured label. | This is the property D8 bought and D13 is designed around. It is also the easiest thing to lose while "just adding one field for the status page". |
| **`entryPosition` still mirrors `exitPosition`.** | Arrival-position spreading would flatten the entry histogram and break D3's transverse continuity. D15 parks it: the dam caused the crowd, and pacing is the answer (§15, A20). Revisit only if the crowding metric still shows a crowd after pacing works. |
| **Velocity and heading are copied, never mirrored** — on both axes. | A north hop is a translation along `y`, exactly as an east hop is along `x`. Nothing about the grid introduces a reflection or an axis swap. |
| **One organism has at most one live `migrationId`** (§6.3). | Two export bands make it possible to write a mod that emits two frames for one organism in one tick. §4.3.2 forbids it; this rule is what it rests on. |
| **No authentication on this wire** (§12 item 1, §14 A17). | M4 is still a LAN milestone with the mod and its sidecar on one machine. The token is Contract B's, and TLS is M5's. |
| **`MIGRATE_OUT_NACK` still carries no `edge`** (§15, A22). | The debt is moot for a new reason, not closed. |

**Enforced by:** everyone, negatively. The test for each row is that the implementation has
no code for it.

### A26 — The corner rule is stated for any declared export set (§4.3.2, §5.3)

*Folded in from the mod implementation, 2026-08-05. Clarifying: no wire change.*

**Change.** A19 wrote §4.3.2 for the only set M4 ships, `["E", "N"]`, and its whole selection
rule was the single comparison `velocity.x ≥ velocity.y`. The mod implements the general form
— a projection onto each edge's outward normal, taken over whatever set is declared — because
`exportEdges` is an array whose contents the mod does not get to assume. The two forms agree
on `{E, N}` and the contract only described one of them, so a third export edge would have had
no defined answer.

**Resolution.** §4.3.2 now states the rule as it is implemented:

| Rule | Statement |
|---|---|
| The score | `outward(v, e) = v · n(e)`, with `n(E) = (1,0)`, `n(N) = (0,1)`, `n(W) = (−1,0)`, `n(S) = (0,−1)`. One expression covers all four edges, and §4.3.1's outward-direction test is `outward(v, e) > 0` — the same quantity, not a second one. |
| The winner | The candidate with the **largest** `outward(v, e)`. |
| The tie | The **earlier edge in canonical order `E, N, W, S`**. The mod sorts its declared set into that order at configuration time and keeps the incumbent on an exact tie, so the ordering *is* the tie-break and there is no separate branch. |
| Reduction | On `["E", "N"]` this is `"E"` when `velocity.x ≥ velocity.y`, otherwise `"N"` — **byte for byte the rule A19 gave**, reproduced and not replaced. |
| **Still no epsilon** | Unchanged, and A19's reasoning is unchanged with it. The comparison is a strict `>` between two floats read from one `Rigidbody2D` in one tick. §4.3.2's `No epsilon` row still applies verbatim. |

**Why canonical order and not, say, declaration order.** Declaration order is an operator's
environment variable, so it would make the corner rule depend on how a slot was configured —
two mods with the same edges in a different order would answer a corner differently, which is
the exact failure §4.3.2 exists to prevent. Canonical order is a property of the contract.
The mod sorting its own set on read is what makes the two statements the same statement.

**What this does not authorize.** M4 declares `["E", "N"]` and nothing else. A26 defines the
answer for a larger set; it does not make one legal. §15 A18's rules still hold — at least one
member, no duplicates, every member also in `borderEdges` — and neither side rejects a set
like `["E", "S"]` at `CONFIG_UPDATE`, so a misconfigured rig is refused one organism at a time
rather than at startup. That is an open item, recorded in §12.

**Enforced by:** the mod, for the projection, the canonical sort and the strict comparison.
The sidecar checks only that `MIGRATE_OUT.exitEdge` is a member of the declared set (§5.3); it
neither knows nor re-derives which band won.

### A27 — An entry-edge portal is visible while the mod's Contract A session is up (§4.2, §5.1, §5.4, §14 A11)

*Folded in from the mod implementation, 2026-08-05. Records a behaviour this document did not
specify at all.*

**The gap.** §14 A11 makes an entry edge **passive**: it is never listed in `EDGE_STATUS`,
never opened and never closed, because it accepts an inbound organism unconditionally and has
no lane state to report. That is right for the wire and it leaves the mod with a question the
wire cannot answer — **when should the player see an entry portal?** An export portal follows
`EDGE_STATUS`. An entry portal has no `EDGE_STATUS` entry to follow, so a mod author reading
this document found nothing, and "nothing" is how two installs end up drawing different
worlds out of the same map.

**Resolution — the defined behaviour.** An entry-edge portal is drawn **exactly while the
mod holds an established Contract A session on the current connection**: the transport is
connected *and* the mod has sent its `CONFIG_UPDATE` on that same connection generation. It is
**never** gated on `EDGE_STATUS`, on an epoch, on a peer's liveness, or on any sidecar reply.

| Condition | Entry portal | Export portal |
|---|---|---|
| Contract A session established | **shown** | shown only when `EDGE_STATUS` reports that edge `open` |
| Session established, every export edge closed with `no_peer` | **shown** | hidden |
| Transport connected, `CONFIG_UPDATE` not yet sent on this generation | hidden | hidden |
| Transport down, or reconnecting | hidden | hidden |

Four preconditions sit in front of that gate, and all four are the mod's own, not the wire's:
the portal is enabled in configuration; a world is loaded; the portal built successfully; and
the edge is a declared entry edge — the opposite of a declared export edge (§14 A11). A
build failure tears the portal down for the session and is reported, never retried silently.

**Why the connection and not the arrival rate.** An entry edge accepts an organism the moment
one arrives, and nothing else about it is observable. Drawing it on arrivals would make a
quiet lane look like a closed one, and drawing it always would promise a door that nothing can
come through while the mod is disconnected. The session is the honest statement: **the door
works exactly while the mod can be handed something.**

**Why this is asymmetric with the export portal, deliberately.** The export portal shows
whether *this world can send*, which is a fact about the map and therefore `EDGE_STATUS`'s to
report. The entry portal shows whether *this world can receive*, which is a fact about the
process. The asymmetry on screen is the asymmetry A11 put on the wire, and a player who sees
one edge glowing while the other is dark is reading the topology correctly.

**Enforced by:** the mod alone. No sidecar behaviour changes, no frame changes, and a
conforming sidecar can neither observe nor influence any of it. It is recorded here because
Contract A is where the edge model lives, and an unspecified behaviour on a specified model is
this document's gap to close.

### A28 — The entry point clamps the free coordinate too, so no arrival lands in a capture band (§4.3, §5.7, §14 A11, §15 A19)

*Folded in from the mod implementation, 2026-08-05, by the M4 pre-flight pass. The code wins:
`BorderGeometry.EntryPoint` already does this, and this document described a geometry the mod
does not produce.*

**The gap, and it is arithmetic.** §4.3 inset the **fixed** coordinate of a spawn by
`W + margin` and took the **free** coordinate straight from the inverse table, unbounded
inside `[−S, +S]`. Three sentences elsewhere then reasoned about what that geometry produces,
and all three are unreachable under the mod's actual formula:

| Claim, before this amendment | Why it cannot happen |
|---|---|
| §4.3: "a bounce-back still lands **inside** its own capture band" | A bounce-back on `E` spawns at `x = S − W − margin`. The band starts at `x = S − W`. `margin ≥ 5`, so the spawn is strictly inside the band's inner boundary, never past it. |
| §5.7: an arrival through `W` "at `entryPosition ≥ (S − W + margin) / 2S` lands in the north band" | Two errors at once. The stated threshold inverts to `y ≥ (S − W + margin)/2 − S`, roughly the middle of the world, not the band. And the real threshold, `position ≥ 1 − W/2S`, is unreachable anyway: the free coordinate is **clamped** to `S − W − margin`. |
| `contract-b-m4.md` §9.4: "The organism therefore lands inside its own capture band" | The same bounce-back arithmetic as the first row. |

**Resolution — state what the code does, and re-derive the rule that depended on it.**

1. **The clamp is normative.** `EntryPoint(edge, position)` computes
   `free = 2S · clamp01(position) − S`, then clamps `free` to `[−(S − inset), +(S − inset)]`
   with `inset = W + entryMargin`, and only then places the fixed coordinate at
   `±(S − inset)`. §4.3's table carries this. The mod owns the whole computation; the
   sidecar still never converts a normalized position and still needs to know none of it.
2. **Why the clamp exists**, and it is not the immunity window's reason: a `position` near
   `0` or `1` is a corner arrival, and without the clamp it would spawn inside the strip of
   the two edges perpendicular to its entry edge — which is a real defect on a grid, where
   those perpendicular edges carry bands.
3. **The guarantee, stated as a guarantee.** Every spawn coordinate, fixed and free, on both
   axes, is at most `S − W − margin` from the origin. A capture band begins at `S − W`
   (§4.3.1). **Therefore no arrival of any kind — ordinary, either axis, or a bounce-back on
   its own export edge — is inside a capture band at the instant it spawns.**
4. **The entry-immunity window keeps its REQUIRED status, on the correct justification.**
   The guarantee is about one instant, and the window is about the ticks after it. A
   bounce-back arrives with its original velocity, which by construction points **outward**
   through the edge it left by, so it re-enters that band under its own power within a few
   `FixedUpdate`s; a corner arrival sits just inside the other axis's boundary and crosses it
   as soon as it travels that way. Without the window a spawn and an export land in adjacent
   ticks and one hop is indistinguishable from two. **The window is what separates the two
   events; the inset is what guarantees they are not the same event.**

**What does not change.** No field, no enum, no message, no default — `contract-a/2.0` is
unchanged and no implementation moves. `entryMargin` keeps its `max(5, 0.5·W)` default and
`entryImmunitySeconds` keeps its 5 simulated seconds. This is a documentation correction plus
one previously-undocumented line of geometry.

**Enforced by:** the mod, in `BorderGeometry.EntryPoint`. The sidecar cannot observe it,
which is precisely why it had to be written down: a second mod implementation reading §4.3
before this amendment would have produced a different world out of the same `MIGRATE_IN`.

### A29 — A stalled mod is paused into, not flooded into: the livelock defences (§5.2, §5.7, §7.3, §7.5, §8, §10)

*Folded in on 2026-08-06, from the slot-6 incident. Five rules, none of them a wire change.*

**The failure, because the rules that follow only make sense against it.** Slot 6 accepted
~10,800 migrants in a morning; its paced backlog pinned at `inboundQueueMax` (64). At 14:39
one `FixedUpdate` spent >3.5 s applying deliveries, `Update` — and with it the §8
heartbeat — never ran, the sidecar closed `4004` on schedule, and §7.5's unconditional
replay re-delivered the same 64 into the next `FixedUpdate`. The mod's dedup ledger had
been cleared by the reconnect, the flood-spawned organisms had already starved, so the
`entityId` scan missed too and every entry re-ran the **full** spawn. Each generation
re-created the stall that caused it; eleven generations later the world was pegged at ~1
TPS with 50 organisms alive, while the upstream sender re-forwarded every stuck entry every
`forwardRetryMs` forever — 203,735 duplicate deliveries, each paying a full decode. Every
individual rule behaved as written.

**The five defences.** Each breaks a different link; any one of them would have degraded
the cycle, and all five together are why it cannot re-form:

1. **The ingest budget** (§5.7). The mod applies deliveries under a per-`FixedUpdate`
   budget and parses frames under a per-`Update` budget, strictly in order, at least one
   per tick. No burst, whatever its depth, can starve `Update` — so the heartbeat cadence
   survives the load that used to kill it, and `4004` is reserved for a mod that is
   actually gone. `pendingIn` (§5.2) counts the whole deferred backlog so pacing stays
   honest. **Enforced by: the mod.**
2. **The durable dedup ledger** (§7.3). The `migrationId` ledger survives a socket
   reconnect and clears only on a `sessionId` change. A replayed handled delivery — even
   one whose organism has since died — is an O(1) hit answered `duplicate: true`, never a
   re-spawn. Replay converges instead of amplifying. **Enforced by: the mod.**
3. **The quiet-mod gate** (§7.5, §8). The sidecar holds `MIGRATE_IN` release once the
   newest `HEARTBEAT` is older than `heartbeatDeliveryGraceMs` (1500 ms). The previous
   arrangement — `pacingIdleGraceMs` (10 s) far beyond `heartbeatTimeoutMs` (3.5 s) —
   meant the pause branch could never win the race against the close: the sidecar
   force-fed a dying connection right up to the `4004`. It delays, never drops.
   **Enforced by: the sidecar.**
4. **The session-scoped bucket and the escalating replay delay** (§7.5). The pacing bucket
   is no longer reset on every socket connect — a same-session reconnect is the same world
   clock, and the burst it would refund is exactly the flood pacing exists to prevent. The
   replay batch itself waits one order-preserving delay that climbs with its
   least-delivered `attempt`, so a batch that keeps forcing `4004` backs off a little more
   each generation instead of arriving harder. Replay stays unconditional — delayed is not
   skipped. **Enforced by: the sidecar.**
5. **The upstream half lives on the other wire** — dedup-before-decode and the
   live-destination retry backoff are `contract-b-m4.md` §14 B8's, and they are what
   removed the 203k-duplicate CPU tax that kept the stalled world stalled.

**What a second implementation must know.** Rules 1 and 2 are mod-law: a mod that drains
its whole queue in one tick, or wipes its ledger on reconnect, satisfies every pre-A29
sentence of this document and still livelocks exactly as slot 6 did. Rules 3 and 4 are
sidecar-law with named defaults in §10 (`heartbeatDeliveryGraceMs`, `replayDelayStepMs`,
`replayDelayCapMs`). None of the five changes a message, a field, an enum or the custody
chain: `4004` still closes at 3500 ms, replay is still unconditional, pacing still runs on
simulated time and still never reorders, and the spawn is still the proof of delivery.

**Enforced by:** both sides, each for its own half — which is the point. The livelock was
a composition of correct parts, so the defence had to be composed too.

---

## 16. Species-identity amendments (`contract-a/2.1`, 2026-08-07)

The owner ratified **Option A — species identity travels in the migration envelope** on
2026-08-07, from the decompiled-source research into how the game binds a restored organism to
a species. **Four amendments, A30 to A33.** A30 puts one OPTIONAL block on two messages; A31
states what the importing mod does with it, before the restore; A32 states what an import
*without* it does instead, and makes that the floor for every import; A33 applies §3.1's
version test.

The section follows the pattern of §13, §14 and §15. Each amendment names the gap or the
change, the resolution, and **which side enforces it** — the side whose code makes the rule
true, and which therefore has to change if the rule changes. Where an amendment contradicts
the body or an earlier set, the amendment wins, and the body carries an `(amended — §16, Ax)`
or `(added — §16, Ax)` marker at that point.

**This set changes the wire, and it is additive.** Two OPTIONAL fields, no removal, no type
change, no enum change, and the nine message types are untouched — so §3.1's own rule answers
with a **minor** bump. The identifier is `contract-a/2.1`, the URL path stays
`/contract-a/v2`, and a peer on either side that speaks `contract-a/2.0` stays compatible by
construction (A33). `contract-b-m4.md` §15, B9 and B10, carry the matching set for the other
wire.

**Enforcement sits on the mod, at both ends**, and that is unusual enough to say once up
front. The sidecar, the relay and the archive carry the block, validate its shape, record it,
and never read a meaning out of it — it is envelope metadata beside the blob, exactly like
`exitEdge`, and D4's boundary is where it always was. Every decision in this set — what to
send, what a name means, which local species an arrival joins — is taken inside a game process
by the mod that owns that world's registry, because that registry is the only place the
question has an answer.

### The defect this set closes

A `.bb8` payload carries exactly one thing about species: `genes.speciesID`, drawn from the
**world-local** counter `Species.speciesMaxID` (`Species.cs:19, 89`). The name is not in the
payload at all — it is `genericName + " " + specificName` on the `Species` record in the
source world's `GlobalLineageManager.recordedSpecies` (`Species.cs:85`), and a world's
registry is its own. So the integer that crosses means nothing at the destination, and
`BibiteGenes.LoadState` looks it up **in the local registry anyway**
(`BibiteGenes.cs:581-583`):

| The incoming id, at the destination | What the game does | What it produces |
|---|---|---|
| **Collides** with a local `speciesID` | The organism silently joins that local species. **No genetic check runs on this path.** | A migrant filed under an unrelated species — and a locally extinct species revived by an organism that has nothing to do with it. |
| **Misses**, and the migrant is genetically near a local species | `gene.species` stays null, and `ResumeBody` → `CheckNewSpecies` joins the nearest active species inside `usedSpeciesSpan` (`BibiteBody.cs:477-480`, `GlobalLineageManager.cs:189-203`) | The best answer available without a name, and the one A32 makes the floor. |
| **Misses**, and the migrant is genetically far | `CheckNewSpecies` creates a species with a fresh **random** local name and no parent | An orphan **root**. `absoluteKeep` is true for any parentless species (`Species.cs:71-77`), so pruning can never remove it. |

About **85% of every rig world's species tree is orphan roots of the third kind** — and of the
two failures, the **first** is the worse one: an orphan root is uninformative, while a
collision is silently *wrong*, and no log line marks either.

Note what already exists and is not reachable from the restore path. The game has a
find-by-name-else-create resolver — `GlobalLineageManager.FindSpeciesFromTemplate`
(`:267-306`) — which matches `s.name == template.speciesName` and creates on a miss. It serves
the **template** path, the editor and the spawn menu, and a live restore never reaches it. A31
is that resolver, run where the restore can see it, on a name the wire now carries.

### A30 — `MIGRATE_OUT` and `MIGRATE_IN` carry an OPTIONAL species block (§5.3, §5.7, §9.1, §9.2, §9.3)

**Change.** Both messages gain one OPTIONAL object, `species`, carrying the migrant's species
**name** and, when there is one, its immediate parent species' name. `MIGRATE_OUT` is where
the mod reads it out of the live world; `MIGRATE_IN` is where the destination mod is handed
it. Nothing between the two interprets it.

**Resolution.**

| Rule | Statement |
|---|---|
| Shape | `{ "genericName": string, "specificName": string, "parentGenericName"?: string, "parentSpecificName"?: string }`. Four fields, no nesting, no id of any kind. |
| The source | The **live `Species` record**, `BibiteGenes.species`, read on the main thread in the same `FixedUpdate` as the migrant's serialization. Never the payload: `$.genes.speciesID` is a counter value, and there is no name in the blob to read. |
| The name | `genericName + " " + specificName`, one U+0020, which is the game's own `Species.name` (`Species.cs:85`). Both halves are REQUIRED when the block is present, non-empty, at most 64 UTF-8 bytes, untrimmed and unfolded. |
| The parent pair | **All-or-nothing.** Both parent fields or neither; one alone is a malformed block. Present exactly when `Species.parentSpecies` is non-null at the source. |
| The depth | **One generation.** A grandparent never travels, and no field exists for one. |
| Optionality | An absent block is **valid** — an organism with a null species record, or a mod that does not implement §16 at all. §5.7's absent-block rule (A32) covers it. |
| Malformed | **Dropped whole, never fatal, and never partially applied** (§5.3, §5.7, §9.1, §9.3). On `MIGRATE_OUT` the sidecar strips a block that breaks these rules, logs one line, and forwards the migration without it; on `MIGRATE_IN` the mod treats one as absent and logs it as a sidecar defect. This is the one named exception to §9.3's "a `data` field fails validation → `MALFORMED_MESSAGE`". |
| Opacity | Sidecar, relay and archive perform **schema validation only**. They **MUST NOT** synthesize, translate, trim, case-fold, normalize or reorder a name, and they **MUST NOT** route, filter or admission-control on one (`contract-b-m4.md` §15, B9). |

**Why names and not an id.** An id is world-local by construction — that is the whole defect.
Any cross-world identifier would have to be minted and agreed somewhere, which means a
registry, an allocator and a consensus problem; and the game already has a global namespace
for species that players read, argue about and screenshot. The name is the identifier the
domain actually uses.

**Why one generation and not the chain.** The importer can only link under a parent that
exists **locally**. A deeper ancestor that is not in the local registry cannot be linked to
anything, and creating it would fabricate a species record no organism in that world ever
occupied — inventing tree nodes to hold a tree. One link is what the destination can honestly
use, and a chain rebuilds itself one link at a time as its members migrate.

**Why it is optional rather than REQUIRED.** Three states produce a frame with no block — an
old mod, an organism whose `BibiteGenes.species` is null, and a block the schema check
stripped — and an importer cannot tell them apart, so making the field REQUIRED would buy no
information and would turn each of those states into a refused organism. A32 gives all three
the same, defined, better-than-today behaviour instead.

**Enforced by:** the mod for reading and writing the block; the sidecar for validating its
shape, stripping a bad one, and carrying a good one without interpretation.

### A31 — The importing mod resolves by name and rewrites `genes.speciesID` before the restore (§1 D4, §4.6, §5.7, §11.2)

**Change.** The destination mod no longer lets a foreign integer reach the game's
deserializer. With a `species` block in hand it resolves the species against its **own**
registry by name, creating one when the name is new, and rewrites `$.genes.speciesID` in the
payload to the **local** id before calling `LoadBibiteOrEggFromData`. §5.7 step 3 states the
mechanism in full.

**The policy, stated as policy so nobody has to infer it: an exact name match is the same
species.** Two organisms whose worlds spell the name identically join one local species. This
accepts a rare **false merge** — the game draws both halves of a name from one shared
`latinNames` list and only checks uniqueness *within* a world
(`SpeciesNameGenerator.RandomUnusedGenus` / `RandomUnusedSpecific`), so two worlds
occasionally issue the same name to unrelated lineages; the observed rate is **1 to 4 names
per world pair**. The owner accepts it, and the reasoning is a comparison, not a hope: the
alternative to a name is genetic distance measured across worlds, which needs a threshold that
is meaningful across two independently drifting populations — a much larger and much less
honest guess than a name collision at that rate. A false merge also stays visible: the merged
species' genetic spread widens where a true one's does not.

**Why before the restore and not after.** `BibiteGenes.LoadState` binds `gene.species` from
the blob it is handed, in the middle of the game's own deserializer
(`BibiteGenes.cs:581-583`). Correcting the binding afterwards means moving a live organism
between two `Species` records — `RemoveFromSpecies` here, `AddToSpecies` there, plus the
counters and the events that hang off both — for every arrival. Rewriting one number in a JSON
string before the call makes the game's own deserializer produce the right answer the first
time, and leaves exactly one code path for a bound species instead of two.

**Why this does not breach D4.** The mod addresses a **named path** and writes it. It does not
parse the blob for meaning, does not deserialize it into a typed model, and reads nothing back
out of it — the incoming value is a foreign counter and there is nothing in it to learn. This
is the same class of operation as the eight position numbers §5.7 step 3 has rewritten since
M2, and §1's D4 row and §4.6 now say so rather than implying a purity the contract never had.
The genome hash is unaffected: `genes.speciesID` is excluded from the canonical projection
(`genome-hash.md` §4.3), so the rewrite cannot invalidate a `lineage.genomeHash` the source
sidecar already computed.

**What this structurally eliminates.** On every import that carries the block, the
ID-collision row of the table above **cannot occur**: the id `LoadState` reads is a local id
the mod just resolved or created, so the lookup either finds the intended species or the mod
created it a moment earlier. The silent misfiling is not made rarer — it is made unreachable.

**What it does not eliminate, stated plainly.** A migrant whose species is new to this world
and whose parent species is not local still becomes a **root**, and a root is still
unprunable. What changes is the count and the label: **one** root per species per world, named
the same as it is named everywhere else, instead of one freshly-random-named root per arrival.
The 85% figure is a population of duplicates, and this is what collapses it.

**Enforced by:** the mod, entirely, at the destination. No sidecar can observe the resolve, and
no sidecar may attempt it — the registry is in the game process.

### A32 — The absent-block rule: no import path keeps a foreign `speciesID` (§5.7, §9.2)

**Change.** A `MIGRATE_IN` with no `species` block is valid and always will be, so the
importer needs a defined behaviour for it. It is **not** "restore as before": restoring as
before is what feeds a foreign counter value into a local lookup.

**Resolution.** The importing mod **MUST remove `$.genes.speciesID` from the payload** before
it restores. Removal, specifically — not a substituted id, not a sentinel, not a value chosen
to be absent locally:

| Why removal, and not a neutral id | Statement |
|---|---|
| The game's own guard | `BibiteGenes.LoadState` runs its lookup under `if (state["speciesID"] != null)` (`BibiteGenes.cs:581-583`). An absent key skips the lookup and leaves `gene.species` null, which is exactly the state the classifier expects. |
| What then runs | `ResumeBody` calls `CheckNewSpecies` (`BibiteBody.cs:477-480`), and the game classifies the arrival by its **own genetic distance** — nearest active species inside `usedSpeciesSpan`, else a new species. That is the game's honest answer, and it is only available once the foreign integer is gone. |
| Why not an "absent" id | Any integer is a claim that could come true. A world mints species ids from a monotonic counter (`Species.speciesMaxID`), so an id chosen to be absent today is an id some later local species is issued — and the defect returns silently, in a world that has been running long enough to reach it. |

**The invariant this creates, and it is the one to remember: no import path retains a raw
foreign `speciesID`.** With a block, A31 overwrites the key with a local id. Without one, this
rule deletes it. There is no third branch. A mod that hands the origin's integer to
`LoadState` is defective under `contract-a/2.1`, whatever its version claims.

**Failure falls this way too.** A resolve or create that throws falls back to this rule and the
organism still spawns (§5.7, §9.2). Custody outranks bookkeeping: a name is recoverable on the
next hop; an organism refused at the door is not.

**Enforced by:** the mod, at the destination. It is also the only part of this set an old
sender can trigger, which is why it is written as a floor rather than as a fallback.

### A33 — `contract-a/2.1` is a minor bump, and the path does not move (§2, §3, §3.1)

**Change.** §3.1: additive fields raise the **minor**; field removal, type changes and
enum-value removal require a **major**. A30 adds two optional fields and removes nothing.

**Resolution.** Apply the contract's own test, item by item:

| Change to Contract A | Kind | Needs a major? |
|---|---|---|
| `MIGRATE_OUT.species` added | additive OPTIONAL field | no |
| `MIGRATE_IN.species` added | additive OPTIONAL field | no |
| Pre-restore resolve, and the `$.genes.speciesID` rewrite | mod-internal behaviour, no wire shape | no |
| The absent-block rule | mod-internal behaviour, no wire shape | no |
| Message catalogue, enums, close codes, NACK codes, tunables | **all unchanged** | no |

The identifier is **`contract-a/2.1`**. By §3.1 the URL path is major-scoped, so it stays
**`/contract-a/v2`**, and `/contract-a/v1` keeps answering with close `4000` exactly as A23
left it. **The minor is a capability statement, not a negotiation** — a receiver detects this
feature by the presence of the `species` field and never by arithmetic on the minor.

**What a mixed rig does, honestly:**

1. A `contract-a/2.0` **sidecar** between two `contract-a/2.1` mods ignores the unknown field
   (§3.1) and forwards a frame without it. The destination mod sees no block and applies A32's
   floor. The degradation is quiet, defined and still better than pre-§16 behaviour.
2. A `contract-a/2.1` **sidecar** and a `contract-a/2.0` mod interoperate unchanged: the mod
   sends no block and ignores the one it is sent. Neither side may reject the other over a
   minor.
3. A **pre-A32 mod at the destination** is the one place a foreign `speciesID` can still reach
   `LoadState`, and no rule on this wire can prevent it — it is an old build's behaviour, not a
   supported configuration. The rig upgrades in one step, as it did at A23, and for the same
   reason: every binary in it is the owner's.

**Enforced by:** both sides, symmetrically. Each sends `"contract-a/2.1"` and compares only the
major.
