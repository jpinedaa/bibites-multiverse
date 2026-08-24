# Contract B — Sidecar ↔ Relay ↔ Sidecar ↔ Archive Wire Specification

**Version:** `contract-b/4.2` (amended — §31, B46; `contract-b/4.1` before it, §25 B37)
**Amended:** 2026-08-05, from the Go implementation (commit `823a70f`). Four resolutions are
folded into the body and recorded in **§14** — **B4** the missing `statsBroadcastIntervalMs`
default (§6.5, §12), **B5** the retry a held entry must keep running (§9.2, §9.3 — **superseded — §25, B37**), **B6** the
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
**Amended:** 2026-08-07, amendment set `contract-b/3.5 + B18–B19` (**§19**), from the owner's
ratification of **the Species and Settings tabs on the site**. The **peer stats block**
(§6.3.1) gains the five mod settings `contract-a.md` §19 puts on `CONFIG_UPDATE` — copied from
the handshake and republished blind in `PEER_STATUS` — plus two version strings the sidecar has
always held and never published: the peer's `modVersion` and the `contractAVersion` its mod
session negotiated. Seven additive OPTIONAL fields, so §4's own test answers with a **minor**
bump to `contract-b/3.5`. Contract A's matching set is `contract-a.md` §19, A42–A44. **They are
read-only**: a control surface is owner-ratified as later work and would be a separate design,
never an extension of these fields (`contract-a.md` §19, A43; B19). Affected body text carries
an `(amended — §19, Bx)` or `(added — §19, Bx)` marker, and **§19 wins over the body and over
§14 to §18 wherever they disagree.**
**Amended:** 2026-08-11, amendment set `contract-b/4.0 + B22–B32` (**§22**), from the M5
decisions the owner ratified on 2026-08-10 and refined on 2026-08-11 — **D21** the per-peer
credential, **D22** layered version compatibility, **D24** the bounded hosted run
(`system_decomposition.md`; `m5_considerations.md`, *Decisions for the Owner*). §3.1's **one
shared token is replaced** by a per-peer credential **bound to the `peerId`**; the transport
becomes TLS; a published capacity table arrives as §3.3; the archive becomes an **authorised**
subscriber with a stated visibility boundary; release, handover and eviction gain an
authenticated admin path; the relay acknowledges every forward; auto-placement gains a rule
for a peer nobody expected; and the handshake gains a minimum **contract**-version gate that
is a compatibility control and never a security one. Replacing a rule that has an installed
base is not additive, so §4's own test answers with a **major** bump to **`contract-b/4.0`**,
and the URL path moves with it to **`/contract-b/v4`** (B32). Contract A's matching set is
`contract-a.md` **§21, A47–A52**, **authored in the same wave**, which takes that wire to the
**minor** `contract-a/2.4` with its path unchanged at `/contract-a/v2` — one wave, two honest
answers from two documents' own tests. Affected body text carries an `(amended — §22, Bx)` or `(added — §22, Bx)`
marker, and **§22 wins over the body and over §14 to §21 wherever they disagree.**
**Amended:** 2026-08-12, amendment set **B33–B34** (**§23**), from the owner's answer to
`m5_considerations.md`'s **Decision 3** — *the migration ledger is kept forever; genome blobs are
pruned to a horizon*. §10's own closing paragraph demanded this: it states that **nothing here
may evict** and that a retention rule contradicting it is a change to D11 rather than a
configuration of it. The change is narrow by construction — **the ledger's rule is untouched**,
and the **genome store** gains an operator-set retention horizon that is **unset by default**,
which the deployment turns on. The same horizon retires a genome gap whose crossing has aged
past it, which is the drain §21's queue never had (Risk 7). **No message, field, enum, code or
routing input changes**, and the wire consequence was already defined: a pruned hash answers
`unknown_hash` exactly as a hash the holder never had (§6.10). §4's own test therefore answers
**neither major nor minor** and the identifier **stays at `contract-b/4.0`**. Contract A takes
**no** set: no rule of that wire changes. Affected body text carries an `(amended — §23, Bx)`
marker, and **§23 wins over the body and over §14 to §22 wherever they disagree.**
**Amended:** 2026-08-16, amendment set **B35–B36** (**§24**), from the transfer measurement of
2026-08-16 — *seven peer connections consume 82% of the host's monthly allowance, and nothing on
this wire is compressed at any layer*. §3's `permessage-deflate` **MUST NOT** becomes a **MAY**,
negotiated per connection under RFC 7692, so a peer that does not offer it is served exactly the
frames every release before this one sent; and §6.5's stats timer becomes the **only** publisher
a stats arrival may reach, which is what §14's B4 and §22's B29 both already assumed and neither
said. **No message, field, enum value, code, close code or routing input changes**, and no peer
can tell a compressed party from an uncompressed one by anything in a frame — the extension is
settled on the HTTP upgrade, below the envelope this document versions. §4's own test therefore
answers **neither major nor minor** and the identifier **stays at `contract-b/4.0`**, on §23's
precedent. Contract A takes **no** set and keeps its own prohibition: that wire is loopback
between two processes on one machine (`contract-a.md` §2), its bytes cross no network, and this
set does not reach it. Affected body text carries an `(amended — §24, Bx)` marker, and **§24 wins
over the body and over §14 to §23 wherever they disagree.**
**Amended:** 2026-08-17, amendment set `contract-b/4.1 + B37–B38` (**§25**), from the owner's
direction of 2026-08-17 — *remove the hold in the re-forwarding; losing an organism is
acceptable and duplicating one is not*. **B37** deletes the bounded hold, the re-forward and
the automatic timeout bounce: a forwarded frame is written **once**, an entry with no terminal
answer within `forwardTimeoutMs` becomes a `lost` tombstone, and at-most-once carries no
exception at all. **B38** bounds the archive's duplicate set to `archiveDedupWindowMs`, which
was unbounded only because a re-forward could arrive a year later. `lostForwardTotal` is added
and `heldDepth` and `bouncedTimeoutTotal` are **retired with their names reserved**. §4's own
test answers **minor** — no frame changes, both directions of a mixed fleet interoperate, and
the additions are OPTIONAL — so the identifier moves to **`contract-b/4.1`** and
`/contract-b/v4` does not. Contract A takes **no** set and no version change. Affected body
text carries an `(amended — §25, Bx)` marker, and **§25 wins over the body and over §14 to §24
wherever they disagree.**
*(This entry was written on 2026-08-17 by §26's author: §25 landed in the body without one, and
an amendment log that skips a set is a log a reader stops trusting. Every claim in it is
§25's own, restated rather than extended.)*
**Amended:** 2026-08-17, amendment set **B39–B41** (**§26**), from the owner's ratification of
2026-08-17 — *keep the answers forever and the raw lines for a window*. §23 changed §10's
never-evict rule on one of its two halves and left the other alone; this set changes the other,
and does it by **splitting the record into what it says and how it says it**. The
**aggregates** — every species' crossing counts and first and last sighting, every ancestry
edge and the ancestry floor, every lane's cumulative flow, every peer's record count, the
coverage denominator and the record counter — become **durable, kept forever, and the source
every surface reads** (**B39**). The **per-crossing lines** gain a window equal to the genome
horizon, are compressed once closed, and are removed only after an off-host copy is confirmed
(**B40**). **One horizon, now three mechanisms** (§23 B34's rule extended). **B41 was drafted
and withdrawn**: it would have bounded the archive's duplicate set, and §25's B38 reached the
contract first and did it. **The number is spent and MUST NOT be reused**, on the same rule
§25 applied to two retired field names. **No message, field, enum value, code, close code or
routing input changes**, and no peer can tell a windowed archive from an unwindowed one by
anything in a frame: the archive is a read-only subscriber that answers no `GENOME_REQUEST`,
so the window is observable only on its own status page. §4's own test therefore answers
**neither major nor minor** and the identifier **stays at `contract-b/4.1`**, on §23's and
§24's precedent — §25 moved it, and this set does not. Contract A takes **no** set. Affected
body text carries an `(amended — §26, Bx)` marker, and **§26 wins over the body and over §14
to §25 wherever they disagree.**
**Amended:** 2026-08-19, amendment set **B42** (**§27**), from the owner's direction of
2026-08-19 — *we don't want the 24 hour thing; it just forwards, and if it's lost it's lost*.
**B42** lowers `forwardTimeoutMs`'s default from 24 hours to **5 minutes**. The mechanism is
§25's, untouched: nothing is re-sent at the deadline and nothing comes home; only the number
moves, and it moves because 2026-08-19's outages showed that open `sent` entries pin the
export intake (contract A's `inboundQueueMax`) for the deadline's whole length — a day of
refused exports over organisms already written off in every way but the bookkeeping. **No
message, field, enum value, code, close code or routing input changes**; §4's own test answers
neither major nor minor and the identifier **stays at `contract-b/4.1`**, on §26's precedent.
Contract A takes **no** set. Affected body text carries an `(amended — §27, B42)` marker, and
**§27 wins over the body and over §14 to §26 wherever they disagree.**
**Amended:** 2026-08-22, amendment set **B43** (**§28**), from the false-`4007` cascade that
stopped bidirectional lanes. Many source peers could send within their own limits while their
combined migrations made one destination answer above its limit. The relay now admits accepted
`MIGRATION_PAYLOAD` frames to a bounded destination transport queue. It writes them at a rate
derived from the published frame limit. The receiving sidecar also paces ordinary durable ACKs
with outbound payloads. **No message, field, enum, code, close code, Contract B wire schema,
URL path, or routing input changes.** §4's test answers neither major nor minor. The identifier
stays at **`contract-b/4.1`**. Contract A takes no set. Affected body text carries an
`(amended — §28, B43)` marker, and **§28 wins over the body and over §14 to §27 wherever
they disagree.**
**Amended:** 2026-08-23, amendment set **B44** (**§29**), from the live Species-tab genealogy
defect. The archive used one mutable parent edge for each normalized species name. The game can
reuse a name, so later evidence can replace an earlier edge and create a cycle in the derived
tree. The archive now keeps bounded, immutable lineage instances. One normalized name can occur
in separate recorded parent paths. The raw record, the live census, and species resolution in the
importing mod do not change. Roll-up format 3 persists the new fold and requires one ordered raw
rebuild from format 2. **No Contract B message, wire field, enum, code, close code, schema, URL
path, routing input, or custody rule changes.** §4's test answers neither major nor minor. The identifier stays
at **`contract-b/4.1`**. Contract A takes no set. Affected body text carries an
`(amended — §29, B44)` marker, and **§29 wins over the body and over §14 to §28 wherever they
disagree.**
**Amended:** 2026-08-23, amendment set **B45** (**§30**), from the source-side refusal wedge
that left live, populated worlds at 64 outbound custody entries while all four lane rates stayed
at zero. A matched relay `NOT_FORWARDED` with `neverForwarded: true` now starts one bounded,
durable progress chain. The sender tries each compatible same-axis destination at most once.
It bounces once if it exhausts those destinations, reaches `maxReroutes`, or reaches the first
refusal deadline. A missing, false, mismatched, or contradicted proof does not start this chain.
The sidecar also records `sent` before it offers any prepared migration to its socket writer.
Thus, a crash or local enqueue error cannot cause a duplicate retry. **No Contract B message,
wire field, enum, code, close code, schema, URL path, or routing input changes.** §4's test answers
neither major nor minor. The identifier stays at **`contract-b/4.1`**. Contract A takes no set.
Affected body text carries an `(amended — §30, B45)` marker, and **§30 wins over the body and
over §14 to §29 wherever they disagree.**
**Amended:** 2026-08-23, amendment set **`contract-b/4.2 + B46`** (**§31**), from the
at-most-once review of B45. A migration-wide `neverForwarded` value cannot identify a refused
attempt after an earlier destination accepted or refused the same migration. A queue-full relay
NACK therefore gains the OPTIONAL `refusedAttempt` object. It echoes that payload's `destSlot`
and `reroute.count`. The source acts only when both values match its current journal attempt.
Graceful relay drain omits `refusedAttempt`, `neverForwarded`, and `relaySessionId`, so it does
not consume destinations or start a refusal deadline. The relay now reads `reroute.count` only
for this correlation. All other reroute fields stay opaque. This additive field makes §4's test
answer **minor**, so the identifier moves to **`contract-b/4.2`**. The path stays
`/contract-b/v4`, and the version floor does not move. Deploy the 4.2 relay before the 4.2
sidecars. A 4.2 sidecar with a 4.1 relay safely leaves the feature unavailable. Contract A takes
no set. Affected body text carries an `(amended — §31, B46)` marker, and **§31 wins over the
body and over §14 to §30 wherever they disagree.**
**Status:** implementation-ready for M4 as written 2026-08-05 from the ratified decisions
D12–D16 (`system_decomposition.md`), the amended D2, and the work order in
`m4_considerations.md`, *Contract Changes Needed*; extended by D17–D20, ratified 2026-08-07
against the living deployment. **Implementation-ready for M5 since §22** (amended — §22, B32),
written 2026-08-11 from D21–D25 and the M5 rows of `m5_considerations.md`, *Contract Changes
Needed*, and from the work-package order in `m5_tracking.md`. **§22 is the wire M5 ships**, and
it was written before any M5 code, which is WP1's whole reason to exist.
**Supersedes:** `contracts/contract-b-m3.md`, in full. That document is the historical
record of the M3 ring and is **not** current guidance.
**Companion documents:** `contracts/contract-a.md` (`contract-a/2.4`, mod ↔ sidecar — the
minor its own M5 set — **that document's §21, A47–A52** — takes for the bearer token, authored in the same wave;
amended — §22, B32) and `contracts/genome-hash.md` (`bb8-genome/1`, the canonical genome projection —
**unchanged by M4, by the species block, by the census and by `contract-b/4.0`**, none of
which is hashed, and whose one payload key, `genes.speciesID`, that projection already
excludes: §4.3 there).
**Title and filename, which stopped agreeing at §22** (amended — §22, B32). This document was
written as *Contract B — M4* and specified M4's wire; since `contract-b/4.0` it specifies
**M5's**, and a title that still said *M4* would be false on its first line.
**`contracts/contract-b-m4.md` stays the path**, because every document in this project cites
it by that name and a rename would break citations to buy tidiness. The second box below
applies the same three tests that split `contract-b-m3.md` off and answers *in this file*.

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
>
> ### And why `contract-b/4.0` was taken here rather than in a successor (added — §22, B32)
>
> The M5 major is the second this document has seen and the first taken **in place**. The
> same three tests answer the other way, one by one:
>
> 1. **Is the file milestone-named and milestone-scoped?** The *file* is; the **document** no
>    longer is. §22 retitles it to the interface it specifies and leaves the path alone, for
>    Contract A's reason (§15 there): the version identifies the wire, the file identifies the
>    interface, and every other document in this project cites this one by name.
> 2. **Is the change structural?** No. §3.1's auth rule is **replaced**, which is what makes
>    the version a major — and it is one row of one table. Everything else in §22 is
>    additive: one new message (`FORWARD_RECEIPT`), one new section (§3.3), new OPTIONAL
>    fields, new refusal *reasons* on existing codes, and rules written down where they were
>    missing. **The message catalogue survives**, the envelope survives, custody, dedup, the
>    hold, the fan-out, routing and hashing are untouched. Half the catalogue's semantics
>    moved in M4; none of them moves here.
> 3. **Is the old text still needed as it stands?** No. Unlike M3's ring, M4's wire has no
>    exit-test record that a `contract-b/4.0` reading would falsify: the M4 record cites this
>    file, and §14–§23 preserve every earlier identifier beside the marked body text, which is
>    exactly how a reader who needs `contract-b/3.5` finds it.
>
> **A major version bump is not by itself a reason to split a file**, and this is the second
> time this project has said so — `contract-a.md` §15's own box said it first, for
> `contract-a/2.0`, and that document has carried three minors since without a copy.

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
slots on either axis; slot handover; an explicit relay answer that proves non-delivery;
~~the bounded hold and its automatic bounce~~ — **superseded — §25, B37**: a forwarded frame is
forwarded once, and an organism that never arrives is lost rather than held; population and
operational stats on the map view (the
"ring view" of D15); and ~~the shared token on the wire, carried unchanged from M3 (D9)~~ —
**superseded — §22, B22**: the shared token is replaced by a per-peer credential bound to the
`peerId`.

**In scope since `contract-b/4.0`** (added — §22): the map stops being a LAN of the owner's
own machines and becomes a **public relay strangers dial** — so TLS (B23), a per-peer
credential (B22), a published capacity table (B24), an authorised archive with a stated
visibility boundary (B27), an authenticated admin path (B28), a forward receipt (B26), a
placement rule for a peer nobody expected (B29) and a minimum **contract**-version gate at the
handshake (B25) all land here. §22 is the whole of it, and `m5_considerations.md` is where each
one's argument lives.

Nothing here caps the map at six. Six is what the rig runs (`m4_considerations.md`,
Question 7); the rules are written for any rectangle, and §13 item 3 records what has not
been tested at a larger one — **and §22's B29 writes the rule that item asked for**, which
churn rather than size is what stresses (added — §22, B29).

Out of scope, and named so nobody builds them by accident: ~~TLS and per-peer authentication
(M5), capacity and abuse limits (M5)~~ — **both are in scope since §22** —
`TOPOLOGY_GOSSIP` and `PEER_EXCHANGE` (M6), `CATALOG_QUERY` and `CATALOG_RESPONSE` (M7), a
**control surface** of any kind (M6 — D23, §13 item 8), and any write interface on the archive
— this wire records and reads only (D11). **Every milestone number in that list is D16's**:
public release is M5, direct P2P is M6, ecosystem completeness is M7.

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
| Protocol | **WebSocket (RFC 6455) over TLS** (amended — §22, B23). ~~over plain HTTP. TLS is **M5** (D9, D16)~~. Plain `ws://` is refused by a public relay and survives only for a loopback rehearsal — B23 states both. |
| URL | **`wss://{relay-host}[:{port}]/contract-b/v4`** (amended — §22, B23, B32). ~~`ws://{relay-host}:{port}/contract-b/v3`~~ |
| Default port | `8795` (moved from M3's `8790` — see *The M4 port plan* below). **A hosted relay behind a name defaults to `443`** (added — §22, B23), because that is the port a stranger's network lets out. |
| Bind address | The relay binds a reachable address, **not** loopback. On the LAN rig the operator opens the Windows Firewall rule for the port and records the host name in `dev_environment.md`; **a hosted relay binds its public interface, or loopback behind a fronting proxy that terminates TLS** (added — §22, B23). |
| Roles | The **relay is the server**. Every sidecar and the archive are **clients** and do all the dialling. |
| Frame type | Text frames. One JSON object per frame. No batching. |
| Encoding | UTF-8, no BOM |
| Compression | **`permessage-deflate` (RFC 7692) MAY be negotiated** (amended — §24, B35). ~~**MUST NOT** be negotiated~~. The relay **offers and accepts** it; **a client that does not offer it is served uncompressed and MUST NOT be refused, ever**. Context takeover is the default on both halves; it costs ~1.25 MB of resident state per connection per process, so a relay carrying hundreds of peers moves to `client_no_context_takeover` / `server_no_context_takeover` rather than to nothing. `wireCompression` (§12) is the relay's off switch |
| Max frame size | 8 MiB (`maxFrameBytes`), same as Contract A |
| Authentication | **A per-peer credential, bound to the `peerId`**, on the HTTP upgrade — §3.1 (amended — §22, B22). ~~A **shared bearer token** … Unchanged from M3~~. |
| Capacity | Published per-peer limits on connections, frames, claims, bytes and genome requests — §3.3 (added — §22, B24). Every one is a knob and every peer-visible one is on the stats block. |
| Reconnect | Exponential backoff with full jitter, `relayBackoffMinMs` to `relayBackoffMaxMs`, the same rule Contract A §6.2 gives the mod. The ladder resets only after a session that stayed up for `stableSessionMs` (`contract-a.md` §13, A8). |

**The path moved with the major**, from `/contract-b/v2` to `/contract-b/v3` — **and again to
`/contract-b/v4` with `contract-b/4.0`** (amended — §22, B32). A relay **MUST** keep serving
every retired path — `/contract-b/v2` and `/contract-b/v3` — and **MUST** close every
connection on one immediately with `4000`, so an older sidecar gets the defined loud error
instead of a bare HTTP 404. The same rule, and the same reason, as `contract-a.md` §15, A23.
**A retired path is served over TLS like the live one** (added — §22, B23): a peer that cannot
complete the handshake learns nothing, and the point of the retired path is that it teaches.

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

### 3.1 The per-peer credential (amended — §22, B22)

**Replaced, not extended** (amended — §22, B22). ~~*The LAN token.* Unchanged from M3 in every
particular: one bearer token for the whole map, held by all six sidecars and the archive,
sourced from `MULTIVERSE_TOKEN` or `--token-file`, compared in constant time, and explicitly
**not** an identity — a token holder could present any `peerId`, including one that already
held a slot, and §3.2's `4006` rule would then evict the legitimate peer.~~ That rule is what
`contract-b/4.0` is a **major** for (B32): it had an installed base, and replacing it is not
additive. What follows is the whole of the new rule; this document inherits nothing silently.

| Rule | Statement |
|---|---|
| Where | The HTTP request that opens the WebSocket carries `Authorization: Bearer <peerId>.<secret>`. Nothing credential-related appears in any frame, ever — **not on `HANDSHAKE`**, which is the first place an implementer will reach for, because a frame is logged, copied to subscribers and forwarded. |
| Who | Every client, and **each one its own**: one credential per `peerId`. The archive holds a credential of its own with a **different grant** (§5.1, B27) — the same mechanism, not a second auth system. |
| The binding, which is the whole security property | The relay **MUST** verify that the `peerId` in the credential is the `peerId` the connection then presents on `HANDSHAKE` (§6.1), and **MUST** refuse the connection when they disagree. A valid credential for `peer-A` presented with `peerId: "peer-B"` is refused **at the handshake**, and **`peer-B` observes nothing at all**: no close, no `4006`, no `PEER_STATUS` change, no `lastRefusal` on its slot. That is the acceptance test, stated so it is testable rather than described (`m5_considerations.md`, Risk 1; D21). |
| Source | `MULTIVERSE_PEER_SECRET`, or `--credential-file <path>` which reads the first line and strips trailing whitespace. A flag that takes the secret literally **MUST NOT** exist — it would put it in every process listing. The `peerId` half is not a secret and comes from `<data-dir>/peer-id` (§7.4) as it always has. |
| Shape | The secret is 32 to 256 bytes of printable ASCII containing no `.`; the RECOMMENDED value is 32 random bytes, hex-encoded. The `peerId` half obeys §6.1's `peerId` rule, so the `.` separator is unambiguous: `[A-Za-z0-9._-]` allows a dot in a `peerId`, therefore the split is on the **last** `.` and never the first. |
| Comparison | Constant-time over the secret (`crypto/subtle.ConstantTimeCompare`), never `==`. The relay **MUST** perform the same work for an unknown `peerId` as for a known one, so that response timing does not enumerate the map's membership — the `peerId`s are on the status page anyway, which is a reason to be tidy here rather than a reason not to bother. |
| Storage | The relay stores a **verifier**, not the secret: a salted hash from which the join string cannot be recovered. A relay whose store is read must not thereby hand over every peer's identity. |
| Issuance | **The relay mints the secret at first claim and prints a join string.** No accounts, no email, no password reset (DQ1). The join string carries the relay URL, the `peerId` and the secret, and it is printed **once**, on the relay's own console, for an operator to hand over out of band. |
| Missing, malformed or wrong | The relay answers HTTP **401** with `WWW-Authenticate: Bearer` and **does not upgrade**. There is no WebSocket, so there is no close code — unchanged from the token rule, and deliberately: a refusal before the upgrade is the one refusal that costs the relay nothing. |
| No credential store configured | The relay **MUST** refuse to start, unless `--insecure-no-token` is passed, in which case it logs one loud warning per accepted connection and **MUST** also refuse to bind anything but loopback. The flag exists for a single-machine test rig. **It is a defaults-audit item for M5's package** (`m5_considerations.md`, decision 7): no installer, script or document may instruct a stranger to pass it. |
| Client reaction to 401 | Retry on the normal backoff ladder and log one loud error each time, naming **the remedy and who must act** — *"this peer's credential was refused; re-run the join string, or ask the relay operator for a slot handover"*. After `authFailuresBeforeCeiling` (5) consecutive 401s, hold the backoff at `relayBackoffMaxMs`. A refused credential is an operator problem and hammering the relay will not fix it. |
| What a refused peer **MUST NOT** do | Generate a fresh `peerId` and connect as a stranger — that strands its slot, its journal's `destSlot` and every organism addressed to it (§7.3 rule 3). Fall back to an unauthenticated connection. Fall back to `ws://`. Try another peer's credential. A sidecar whose credential is refused **keeps its journal, keeps delivering inbound entries to its own mod, and waits for a person** (§7.5). |

**What this buys, and what it costs.** It closes both M4 gaps at once: a `peerId` is now a
**credential rather than a claim**, so §3.2's `4006` eviction stops being a one-frame denial of
service against any peer whose `peerId` can be read off the status page; and B23's TLS keeps
the credential, the genome and the peer id off the wire in clear. **The cost is stated because
it is real**: there is no recovery path in the software. A stranger who loses their join string
loses that world's identity until an operator hands the slot over by name — `--handover-slot`
(§7.5), over the authenticated admin path B28 adds. That is the honest price of *no accounts*,
and it was chosen with the price in view (DQ1).

**One M4 rule kept its shape and changed its reason** (amended — §22, B22, B28). Slot handover
(§7.5) rebinds a reservation to a new `peerId`. Under M4 it could not be a wire operation
because a `peerId` was a claim, not a credential; under `contract-b/4.0` it **may** be one, and
B28 gives it an authenticated path — because the operator whose console it needed is now on a
VPS and the peer that needs it is a stranger. What does **not** change is that it stays a
deliberate act with a printed consequence and a confirmation (§7.5).

### 3.2 Close codes

| Code | Name | Sent by | Meaning |
|---|---|---|---|
| `1000` | `NORMAL` | either | Clean shutdown. |
| `1009` | `TOO_BIG` | either | Frame over `maxFrameBytes`. |
| `4000` | `PROTOCOL_UNSUPPORTED` | relay | The `protocol` **major** version is not supported, or the connection arrived on a retired path (§3). The client **MUST NOT** reconnect until it is restarted. |
| `4003` | `MALFORMED_FRAME` | either | This code covers invalid JSON, a missing REQUIRED envelope field, or no first `HANDSHAKE`. It covers a routing field that disagrees with the sender's peer id. A missing, empty, or invalid `MIGRATION_PAYLOAD.migrationId` also causes this close (amended — §28, B43). A `HANDSHAKE.peerId` that disagrees with the connection credential also causes it (added — §22, B22). An incompatible `gameVersion` (§6.1) or a `protocolVersion` below the published minimum also causes it (added — §22, B25). Reconnect with backoff. The reason string and relay log name the cause. A slot's `lastRefusal` also names applicable handshake causes (§6.5). The credential-mismatch case reaches no slot and appears on none (B22). |
| `4004` | `LIVENESS_TIMEOUT` | relay | No frame and no `PONG` within `peerTimeoutMs`. Reconnect with backoff. |
| `4005` | `SHUTTING_DOWN` | either | The sender is draining. Reconnect with backoff. |
| `4006` | `REPLACED` | relay | A newer connection **that authenticated as the same `peerId`** claimed it (amended — §22, B22). The old connection **MUST NOT** reconnect. **The credential is what makes this rule safe**: under the shared token any holder could take any peer's slot with one frame, and under §3.1 only the peer itself can replace itself — which is the same self-healing rule `contract-a.md` §2 gives the mod socket, with the impersonation removed. |
| `4007` | `CAPACITY` | relay | **New in `contract-b/4.0`** (added — §22, B24). A published inbound limit in §3.3 was exceeded and the relay is shedding this connection rather than the map. The close reason names **which** limit and its value. Reconnect with backoff. A client that takes two `4007`s in a row **MUST** hold at `relayBackoffMaxMs` until an operator or a configuration change intervenes. **A full destination migration queue is not this close** (amended — §28, B43). It returns `NOT_FORWARDED` and leaves the destination connected. |

### 3.3 Capacity and abuse limits (added — §22, B24)

**New in `contract-b/4.0`.** A public relay meets peers nobody vetted, and M4 had no limit of
any kind on this wire beyond `maxFrameBytes` and one compiled-in genome rate. B24 states the
table; the whole of its design constraint is D1: **every limit here is countable at the frame
level**, so none of them requires the relay to read a body it is forbidden to read (§5).

| Limit | Default | Scope | What it counts |
|---|---|---|---|
| `maxConnectionsPerPeer` | `2` | relay, per `peerId` | Simultaneous authenticated connections. The second is the §3.2 `4006` overlap during a reconnect; a third is `4007`. |
| `maxConnectionsPerAddress` | `8` | relay, per source address | Simultaneous connections before the upgrade is refused with HTTP **429**. It is deliberately loose: one machine legitimately runs several peers, and the rig runs five. |
| `maxFramesPerSecond` | `50` | relay, per peer | Frames of any type. A peer at `statsIntervalMs` sends well under one a second. The ceiling is sized for a migration burst, not for a steady rate. The relay **MUST** refuse a configured value below `8`, because B43 reserves one eighth for causally bounded migration replies (amended — §28, B43). |
| `maxFrameBytes` | `8388608` | both | Unchanged (§12). Over it is close `1009`, not `4007`. |
| `maxBytesPerSecond` | `4194304` | relay, per peer | Sustained inbound bytes. It is what stops `maxFramesPerSecond` from being evaded with maximum frames. |
| `maxClaimsPerMinute` | `12` | relay, per peer | `SECTOR_CLAIM` frames. Above it the relay answers `granted: false, reason: "rate_limited"` and does **not** close: a claim storm is usually a peer whose measured time scale is wandering (DQ3's 64 claims in a day), and a refusal it can read beats a close it must recover from. |
| `maxGenomeRequestsPerMinute` | `30` | both, per requester **per answering peer** | The existing `genomeRequestsPerMinute` (§10, §12), **renamed into this table and now a knob**. Above it the answer is `found: false, reason: "rate_limited"`, exactly as today. |
| `maxSubscribers` | `4` | relay | Connected `role: "archive"` clients. B27 makes each one individually authorised, so this bounds the fan-out cost rather than the trust. |

**Every one of them is a knob** (D20). Each has a flag and an environment variable, and none
is a compiled constant — *"a tunable an operator cannot retune from the metric that measures
it is not a tunable."* `genomeRequestsPerMinute` is the worked example of the failure: it
shipped as `contractb.GenomeRequestsPerMinute = 30`, reachable only by editing source, and it
is the limit a public archive is most likely to need to move.

**Every peer-visible one is published**, and that is the second half of D20's rule. The relay
publishes the values **it is running with** — not the shipped defaults above — in
`HANDSHAKE_ACK.limits` at connect and `PEER_STATUS.limits` thereafter (§6.2, §6.5), which is
the same broadcast the stats blocks ride so a page can render one against the other. A peer can
therefore be **built** to respect a ceiling instead of discovering it as a `4007`, and a limit
nobody can read is a support conversation nobody can win. **They are beside the stats block and
not inside it**, and §6.3.1 states why: that block is peer-authored end to end, and a
relay-authored field in it would be the first value on it the peer did not write.

**The destination transport derives its bounds from two published limits** (added — §28,
B43). Let `F` be `maxFramesPerSecond` and `B` be `maxBytesPerSecond`. These values add no
new knob and no new published field.

| Transport rule | Requirement |
|---|---|
| Queue bounds | Each live destination connection has a migration-only transport queue. It retains at most `F` frames and at most `B` bytes. Both bounds apply independently. A selected physical write counts until that write returns. An enqueue that would exceed either bound returns queue-full. This includes one frame larger than `B`. |
| Write rate | The relay writes queued migrations to one destination identity at `R = max(1, floor(F / 8))` frames per second. Writes are evenly spaced. One identity owns the pace across overlapping old and new connections. A reconnect cannot reset it. |
| Ordinary priority | Ordinary and control frames remain sendable while the next migration is not due. After a migration becomes due, at most `7` ready ordinary or control frames may pass it. The due migration then reserves the next physical send slot. |
| Full queue | A full migration queue returns non-fatal `NOT_FORWARDED` to the source. It does not close the destination connection. It does not create or update a forwarding record (§5.2, §6.8). |

**A source handler never waits for this pace.** After validation and routing, it accepts the
opaque frame or immediately returns the applicable relay NACK. Queue-full and drain refusals
use `NOT_FORWARDED`. The handler creates no asynchronous source job and retains no decoded
body. The destination writer owns all later waiting.

**The sidecar uses one shared deferred-send budget** (added — §28, B43). Ordinary,
journal-backed `MIGRATION_ACK` frames and outbound `MIGRATION_PAYLOAD` frames use the same
frame and byte pacing. Pending ACKs drain before new outbound payloads. A deferred frame stays
in durable state and is offered again. Immediate `MIGRATION_NACK` frames and tombstone re-ACKs
bypass this deferred class. Each is the answer to one relay-paced arrival, so their aggregate
rate is causally bounded by `R` (§6.7, §6.8).

**A proven full-queue refusal cannot pin the source at its custody cap** (amended — §31,
B46). Only a queue-full `NOT_FORWARDED` carries `refusedAttempt`. The source requires the
NACK's session, `destSlot`, and `rerouteCount` to match its current journal attempt. It also
requires no contradictory receipt for that session and destination. It then records the
destination, the `refused` transition, and the first refusal deadline in one durable update.
It walks to the next compatible, untried destination on the same axis. A graceful-drain
`NOT_FORWARDED` carries no proof fields. It does not start or advance this walk. A pace
deferral happens before the durable `sent` transition and remains retryable. After pace
admission, the sidecar records `sent` before it offers the prepared frame to the socket
writer. A socket enqueue error is ambiguous and is not retryable (§9.2, §9.3).

**What a limit MUST NOT be.** No limit on this wire may require the relay to decode
`data.body.bb8`, `data.lineage` or `data.species` (§5), to index anything, or to keep
per-organism custody or domain state. The bounded opaque frames in the destination transport
queue are transport state, not an organism index or a journal (amended — §28, B43). A limit
that needs a payload read is a limit this relay may not have. D1 is why the archive is a
separate service and why M6 can replace the relay with libp2p. An abuse limit is not worth
spending it.

**The relay sheds the connection, never the map.** A peer over an inbound limit is closed with
`4007`. A claim-limit breach gets a refusal. Other peer connections remain open. A migration
queue refusal happens before relay acceptance and returns `NOT_FORWARDED`. A connection loss
after acceptance can drop queued frames under §5.2's attempted-write rule. `SLOT_VACANT` still
means what §6.8 says it means. A peer shed for capacity is `live: false` with `darkSinceMs` set
like any other dark peer (§6.5). Its neighbours route around it as they always did (§8).

---

## 4. The envelope

Identical in shape to Contract A §3 — five fields, no more:

```json
{
  "protocol": "contract-b/4.2",
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
| `MIGRATION_NACK.refusedAttempt`; relay reads `MIGRATION_PAYLOAD.reroute.count` for correlation | additive field and additive relay-visible input | no — minor `contract-b/4.2` (§31, B46) |

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

**The world's settings are the fifth, and it is a minor for the fifth time** (added — §19,
B19): the peer stats block gains `modVersion`, `contractAVersion`, `migrationExclude`,
`saveMinutes`, `saveKeep`, `saveOnQuit` and `worldWrapping` (§6.3.1), all seven additive and
all seven OPTIONAL, so the identifier moves to **`contract-b/3.5`**. The degradation is B16's
exactly: a `contract-b/3.4` **relay** carries them because §16's B11 asked it to store the
block as the bytes it arrived as, and any peer that does not know them omits them. **Every
path renders as unknown on the page** (§10.1), and here too "unknown" must beat a plausible
substitute out loud — a page that fills in `saveMinutes: 10` because that is the shipped
default is describing a world whose save timer may well be off.

**`contract-b/4.0` is the second major, and it is a major for one row of one table**
(amended — §22, B32). B32 applies §4's test line by line; the short form is that §3.1's
shared-token rule is **replaced** and there is an installed base, which is the expensive shape
§3.1 and `contract-a.md` §15's A41 both name. Everything else in §22 passes the additive test:
one new message type (`FORWARD_RECEIPT`, §6.12), one new section (§3.3), new OPTIONAL fields
on existing objects, new *reasons* on existing codes, and one new close code. **A
`contract-b/3` sidecar and a `contract-b/4` relay are incompatible by design** and say so with
close `4000` on the retired `/contract-b/v3` path (§3), exactly as `contract-b/2` and
`contract-b/3` did.

**`contract-b/4.1` is the sixth minor, and it is a minor although a RULE was removed**
(added — §25, B37). §25 deletes the bounded hold: a forwarded frame is forwarded once, an
unanswered one is recorded lost, and nothing comes home on a timer. "A rule is being replaced"
is §22 B32's own test for a **major**, so the answer has to be argued rather than asserted, and
§24 argued the same shape one set earlier:

1. **The identifier is a statement about FRAMES, and no frame changes.** No message type, no
   field, no enum value, no NACK code and no close code is removed. `MIGRATION_PAYLOAD`,
   `MIGRATION_ACK`, `MIGRATION_NACK` — `neverForwarded` and `relaySessionId` included — and
   `FORWARD_RECEIPT` all keep their shape and their meaning. What changed is what ONE PEER does
   with ITS OWN JOURNAL.
2. **The replaced rule has an installed base that keeps working, in both directions.** A
   sidecar built before this set still holds and re-forwards; against a `contract-b/4.1` relay
   and archive it produces exactly the pre-§25 wire, and its retries are absorbed where they
   were always absorbed — the destination deduplicates on `migrationId` against its journal and
   its tombstones (§6.6), and the archive deduplicates on the record's key (§5.1, §25 B38). A
   `contract-b/4.1` sidecar against an older relay simply never uses the retry that relay
   expected. **Neither combination loses or duplicates an organism**, which is the property
   §22 B32 priced and the one this set must not spend.
3. **The minor is for the one additive OPTIONAL field**, `stats.lostForwardTotal` (§6.3.1) —
   exactly the test §15's B10, §16's B12, §18's B17 and §19's B19 each answered with a minor.
   **`heldDepth` and `bouncedTimeoutTotal` are RETIRED, not removed**: a `contract-b/4.1`
   sidecar stops sending them, which §6.3.1's *every field is optional, and absence is a value*
   rule has permitted since the block existed, and a reader renders them as **unknown**. Their
   names are reserved and MUST NOT be reused.
4. **A major would do active harm.** B25's `minContractVersion` lets an operator refuse peers
   below a published floor, and the path move a major carries (§3) closes the old path with
   `4000`. Either would evict a world for running last month's build in order to apply a change
   that costs that world nothing — and this project moves its fleet by publication and cannot
   make a participant upgrade (D25). **`/contract-b/v4` does not move.**

**Destination transport pacing does not move the identifier** (added — §28, B43). The
envelope, message catalogue, fields, enum values, close codes, NACK codes, and routing inputs
do not change. Queue admission, write scheduling, graceful drain, and durable ACK retry are
local behavior on existing frames. A mixed pair still exchanges the same bytes. The identifier
therefore stays at **`contract-b/4.1`**, and `/contract-b/v4` does not move.

**Attempt-scoped queue-refusal proof is the seventh minor** (added — §31, B46).
`MIGRATION_NACK.refusedAttempt` is an additive OPTIONAL object. A 4.1 relay omits it, and a
4.2 source then treats the refusal as unproven. A 4.1 source ignores it. Both directions are
safe, but the bounded queue-refusal walk is available only after the relay speaks 4.2. The
relay must therefore be upgraded before sidecars. The path remains `/contract-b/v4`, and the
operator must not raise `minContractVersion` for this feature. The identifier moves to
**`contract-b/4.2`**.

**One rule §22 adds is deliberately *not* a compatibility rule, and the distinction has to be
read here** (added — §22, B25). B25's minimum **contract**-version gate refuses a peer whose
`protocolVersion` is below a published floor — and that floor may sit at a **minor**. That
does not make the minor a rejection reason on this wire: §4's rule is about what two *peers*
may assume of each other, and it is unchanged — the minor is never a rejection reason, unknown
fields and types are ignored, and a feature is detected by the presence of its field. B25's
floor is an **admission policy of one map**, set by its operator, defaulting to *no minimum*,
published where a peer can read it before it dials, and raised only after the release that
satisfies it exists (DQ5, D25). A relay that raises it has changed its own deployment; it has
not changed this contract's compatibility rule.

Timestamps are informational (D5). `messageId` is for log correlation only; `migrationId` is
the one idempotency key in the system (`contract-a.md` §7.1).

---

## 5. Routing, and what the relay may read

The relay reads the routing fields below. Since 4.2, it also reads one correlation field from
a migration payload (amended — §31, B46):

| Field | Present on | Routes to |
|---|---|---|
| `destSlot` | `MIGRATION_PAYLOAD` | The peer that currently holds that slot |
| `destPeer` | `MIGRATION_ACK`, `MIGRATION_NACK`, `GENOME_REQUEST`, `GENOME_RESPONSE` | That peer id |
| `reroute.count` | `MIGRATION_PAYLOAD` only | Echoed with `destSlot` in a queue-full `refusedAttempt`. Absence means the original attempt, count 0. It is not a routing decision. |

Every routable frame also carries `sourcePeer`. The relay **MUST** check it against the
sending connection's own peer id and **MUST** close with `4003` when they disagree. It
**MUST NOT** rewrite it, so the frame can be forwarded byte for byte. A present
`reroute.count` MUST be a positive integer. A zero, negative, or malformed value closes the
source with `4003` (added — §31, B46).

The relay **MUST NOT** decode `data.body.bb8`, **MUST NOT** decode `data.lineage`,
**MUST NOT** read `data.species` (added — §15, B9), and **MUST NOT** validate a payload
(D4 — that is `bb8-schema`'s job, sidecar-side, at both ends). It forwards the original frame
bytes unchanged. A species name is **never** a routing input, a filter, or an admission-control
term. The relay routes on `destSlot` and `destPeer`. It reads `reroute.count` only to
correlate a queue refusal. Every other reroute field remains opaque (amended — §31, B46).

**The relay MAY retain those original bytes only in the bounded migration transport queue**
(added — §28, B43). It does not decode, rewrite, index, transfer, or re-resolve a queued
frame. This is transport buffering under §3.3, not custody. A sidecar journal remains the only
durable organism state on the migration path.

**Routing is on the slot, not on the peer, and not on the position.** `destSlot` is an
address: if the peer holding that slot changed identity since the sender journaled the
migration, or if the map moved that slot to a different coordinate, the frame goes to whoever
holds the slot **now**. This is what makes insertion, re-positioning and handover safe for
work in flight (§7.3), and it is why the coordinate never appears on a migration frame.

The relay **MUST** answer the sender when it declines a migration. No reservation gives
`SLOT_VACANT`. No live connection gives `PEER_OFFLINE`. A full B43 queue or closed drain
admission gives `NOT_FORWARDED` (§6.8, amended — §28, B43). For an offline `destPeer` fetch,
the relay sends `GENOME_RESPONSE` with `found: false, reason: "peer_offline"`. A silent drop
turns a bounded failure into a stall. It also withholds the evidence needed for a re-route
(§5.2).

**The relay computes the effective neighbour, and that is the only new thinking it does**
(D12). Walking a row or a column over the registry it already holds adds no new knowledge to
a deliberately dumb relay. It still parses no body, indexes nothing, and takes no organism
custody. B43 permits only its bounded opaque transport queue (§3.3, §28). §8 states the walk.

### 5.1 The archive is a read-only, **authorised** subscriber (amended — §22, B27)

`multiverse-archive` connects to the relay as a client with `role: "archive"` (§6.1). It
owns no world, holds no slot, and never appears in the structural order.

**It is no longer any client that happens to hold the token** (amended — §22, B27). Under M4
`role: "archive"` was a self-declaration and the shared token was the only gate, so anyone who
could open a socket could subscribe to every envelope on the map. Under §3.1 a subscriber
authenticates as a `peerId` like everybody else, and the **subscribe grant is separate from
the peer grant**: the same mechanism, a different permission.

| Rule | Statement |
|---|---|
| **Authorisation** (added — §22, B27) | A client MAY take `role: "archive"` **only** when its credential carries the **subscribe** grant. A credential without it that asks for `role: "archive"` is refused at the handshake with `4003` and a reason naming the missing grant. A credential **with** it that asks for `role: "peer"` is refused the same way: the two grants are disjoint, so a compromised subscriber cannot claim a slot and a compromised peer cannot read the map's whole traffic. |
| **Granting it** (added — §22, B27) | The subscribe grant is issued by the **relay operator**, deliberately, at the same console that mints a join string (§3.1) — there is no wire message that asks for one and none that confers one. A public map has exactly as many subscribers as its operator decided to have, bounded by `maxSubscribers` (§3.3). |
| **The visibility boundary, stated rather than implied** (added — §22, B27) | A subscriber sees **every** `MIGRATION_PAYLOAD`, `MIGRATION_ACK` and `MIGRATION_NACK` the relay routes or generates, and **every** `PEER_STATUS` — which since §16 carries each world's species census and since §19 its mod version, its `contract-a` version, its save policy and its exclusion list (§6.3.1). **That is a fairly complete profile of a stranger's machine, and it is what the grant grants.** It is stated here so that granting it is a decision rather than a default, and so that D24's participant announcement can say what a participant is agreeing to. What a subscriber does **not** get is anything more than the peers themselves get: every field it reads is a field the relay already broadcasts to all six sidecars, and there is no subscriber-only view, no private field and no back channel. |
| **What a peer may assume about its own stats** (added — §22, B27) | Nothing is confidential on this wire. A peer that must not publish a value **MUST NOT put it on the stats block**, because there is no rule here that would keep it from a subscriber. The block is a publication, and §6.3.1's fields are the whole of what is published. |
| Fan-out | The relay **MUST** send every connected subscriber a **byte-identical copy** of each accepted `MIGRATION_PAYLOAD` (amended — §28, B43). It **MUST** also copy every routed `MIGRATION_ACK` and `MIGRATION_NACK`. The copy carries the original `sourcePeer`, `destSlot` and `migrationId`. |
| Fan-out covers the relay's own non-delivery answers | **The set is a superset of "routed"** (amended — §14, B7). Every `MIGRATION_NACK` the relay **generates in answer to a `MIGRATION_PAYLOAD` it declined to forward** — `SLOT_VACANT`, `PEER_OFFLINE`, `NOT_FORWARDED` (§6.8) — **MUST** also be fanned out. Queue-full `NOT_FORWARDED` adds `refusedAttempt` in 4.2. Graceful-drain `NOT_FORWARDED` omits all proof fields (amended — §31, B46). The relay's two **connection-level** refusals are **not** fanned out, because no migration was in question: `NOT_A_MEMBER` (a payload from a subscriber, refused as a role error) and `PEER_UNKNOWN` (a routed `ACK`/`NACK` whose `destPeer` has gone). |
| Best effort | The fan-out **MUST NOT** delay, block or fail a migration. A subscriber that is absent, slow or dead changes nothing on the migration path. |
| Bounded | Each subscriber has a queue of `archiveQueueMax` (1024) frames. On overflow the relay **MUST** drop the **oldest** copy, increment a dropped-copies counter, and log at most one line per minute. It **MUST NOT** disconnect the migration it was copying. |
| No answers | A subscriber **MUST NOT** answer a copied frame. The relay ignores an `ACK`/`NACK` from a subscriber with one warning. |
| No sending | A `MIGRATION_PAYLOAD` from a subscriber is answered `MIGRATION_NACK` / `NOT_A_MEMBER` and is **not** forwarded. |
| What a subscriber may send | `HANDSHAKE`, `PING`, `PONG`, `GENOME_REQUEST`. Nothing else. |
| No claim | A `SECTOR_CLAIM` from a subscriber is refused with `granted: false, reason: "role_has_no_slot"`. |
| Duplicates | A re-routed migration produces a second copy, and so does a sidecar older than §25's B37 running the retry that set removed. The archive deduplicates a `MIGRATION_PAYLOAD` and a `MIGRATION_ACK` on `migrationId`, exactly as a sidecar does, inside the bounded B38 window. A re-routed copy is the same organism on a new lane (§6.6, §9). A legacy or proof-free `MIGRATION_NACK` deduplicates on `migrationId` + `code` (amended — §14, B7). A 4.2 `NOT_FORWARDED` with `refusedAttempt` adds its `destSlot` and `rerouteCount` to that key (amended — §31, B46). This keeps distinct queue refusals without changing old record keys. |
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
| The record | The relay keeps the set of `migrationId`s it has **forwarded**. It stores the first accepted-forward time for `forwardRecordRetentionSeconds` (172 800 s = 48 h). |
| Validity before admission | A missing, empty, or invalid `migrationId` makes the frame malformed. The relay closes the source with `4003` before queue admission. Thus, the record-to-queue invariant below applies to every accepted frame (amended — §28, B43). |
| What counts as forwarded | **Successful local acceptance into the destination's paced transport queue** (amended — §28, B43). This is the attempted-write boundary. Physical socket writing follows asynchronously. A peer that disconnects with the frame queued is indistinguishable from one that received it, so both count as forwarded. |
| Atomic boundary | Final target validation, forwarding-record insertion, and transport-queue acceptance **MUST** be one atomic relay action under the relay lock (added — §28, B43). A relay proof or routing decision observes both the record and acceptance, or neither. |
| Close race | Queue acceptance and connection close **MUST** have one order. If close wins, the relay creates no new record and returns `PEER_OFFLINE`. If acceptance wins, the record remains and the close follows the after-acceptance rule below. |
| What does not count | A frame does not count if the relay refuses it before queue acceptance. Causes are no slot, no live peer, a full queue, or closed migration admission. |
| After acceptance | A queued frame stays with that exact destination connection. The relay **MUST NOT** transfer it to a replacement connection, re-route it, or send a later NACK for it. A disconnect or process exit may lose it. The forwarding record remains, so the ambiguity resolves conservatively toward at-most-once. |
| The session | The relay mints a `relaySessionId` (a UUID) at process start. It reports it in `HANDSHAKE_ACK` (§6.2) and in proof-bearing relay NACKs (§6.8). The record covers **that session only**. Graceful-drain NACKs omit it (amended — §31, B46). |
| The legacy answer | A proof-bearing relay NACK retains `neverForwarded` for 4.1 compatibility. `true` means the `migrationId` is absent from this session's forwarding record. Otherwise it is `false`. This value is migration-wide and does not identify a mixed-history attempt (amended — §31, B46). |
| The 4.2 queue answer | A queue-full `NOT_FORWARDED` also carries `refusedAttempt`. It copies the rejected payload's `destSlot` and `reroute.count`, using 0 when `reroute` is absent. It proves only that exact queue-admission attempt. Graceful drain carries none of these proof fields (added — §31, B46). |
| Memory | One `migrationId` and one timestamp per forwarded migration. At T1's measured rate — 1 799 hops an hour — 48 hours is about 86 000 entries, a few megabytes. It is in memory, and it is deliberately **not** durable: a relay restart is exactly the event that invalidates the proof, and persisting it would claim knowledge the new process does not have. **The 48 hours used to be sized at twice the sender's hold** (§12); §25's B37 removed the hold, and what the retention now covers is a **re-routed** entry's later attempts and a sidecar older than B37 still retrying. The value does not move. |
| **The receipt** (added — §22, B26; amended — §28, B43) | After acceptance, the relay **MUST** attempt one `FORWARD_RECEIPT` (§6.12) to the **sender**. Queue acceptance establishes the fact and its session. Receipt enqueue is best effort and is not part of the atomic record-to-queue action. A **re-route** of the same `migrationId` produces another receipt attempt. A retry from a sender older than §25's B37 does too. |
| **What the receipt is for** (added — §22, B26) | It moves the forwarding record **into the sender's own durable journal** (§7.4, §9.2). A sender that holds a receipt knows the frame was forwarded under a named session and does not need to ask; a sender that holds **no** receipt for an entry it wrote to a live relay connection knows only that it does not know, which is exactly what `sent` already means. |
| **What the receipt is NOT** (added — §22, B26; amended — §28, B43) | Not a delivery acknowledgement — §6.7's `MIGRATION_ACK` is, and it comes from the destination sidecar after custody. Not custody. Not an answer. **Not proof of non-delivery, and never proof of delivery**: a receipt states that the relay accepted an attempted write. An accepted frame and a delivered frame are indistinguishable. The safe direction of §9.2 is unchanged. A receipt can move an entry only toward leaving it where it is, never toward re-routing. |

**Connection loss and relay drain do not use the same close path** (added — §28, B43).

| Event | Required result |
|---|---|
| Connection error or replacement | Stop admission to that connection. Drain its ordinary queue, but promptly drop its queued migrations. At most one already-selected migration write may finish. Keep every forwarding record. Do not transfer, re-route, or later NACK a dropped frame. The identity's pace survives for any overlapping or replacement connection. |
| Relay-wide graceful drain | First close migration admission. Return proof-free `NOT_FORWARDED` for a later payload without updating its forwarding record. Omit `refusedAttempt`, `neverForwarded`, and `relaySessionId`. Then drain each ordinary queue and all accepted migration frames. Run connection drains in parallel and keep the normal identity pace (amended — §31, B46). |
| Drain deadline | The graceful drain has one relay-wide deadline of **18 seconds**, not one deadline per connection. At the deadline, force-close every remaining connection and drop its remaining queue. Keep the conservative forwarding records. |

The queue holds at most `F` frames and drains at `R = max(1, floor(F / 8))`. For every
supported `F ≥ 8`, the pace portion is at most 15 seconds. The maximum occurs at `F = 15`.
The **18-second deadline is the hard relay-wide boundary**. At that deadline, the relay
force-closes a blocked write and drops every remaining frame. The forwarding records preserve
the existing attempted-forward ambiguity.

**Why the receipt, and why now** (added — §22, B26). §13 item 6 named this fix, priced it at
one frame per migration, and declined it because *"a relay restart is rare and
`--release-inflight` is one command"*. **Both halves of that sentence are properties of a
relay on the owner's desk.** A hosted relay has deploys, certificate rotation, kernel updates
and a supervisor doing its job (D24, DQ2), so restarts stop being rare; and
`--release-inflight` is typed on the **sender's** machine, which after M5 is usually a
stranger's. The condition item 6 set was *"if this ever hurts"*, and the change of venue is
what makes it hurt — knowably in advance rather than after the first bad night.

**What it costs, and where that is measured.** One extra frame per migration on a relay whose
whole design virtue is that it forwards frames and does nothing else (D1). At the rig's
measured 300–500 crossings a minute it is a rounding error; at a public map's rates it is a
real cost, and WP3 **measures it at rate rather than assuming it away**
(`m5_considerations.md`, DQ2). The receipt is deliberately the cheapest possible frame: three
fields, no body, no fan-out to subscribers, and no answer.

**The receipt does not make the record durable, and must not be read as doing so.** The relay
still cannot speak for a session it did not run, and a sender that holds a receipt from
session *X* and hears `neverForwarded: true` from session *Y* has learned nothing about *X*
(§9.2). What the receipt buys is that the sender no longer needs the relay to still be the
same process to know its own entry was forwarded — the fact is in the sender's journal, where
D2 keeps custody.

**An attempt proof cannot contradict a receipt for that attempt** (amended — §31, B46). If a
sender recorded a `FORWARD_RECEIPT` for session *X* and destination *D*, it MUST NOT accept a
queue-refusal proof for session *X* and destination *D*. It keeps the entry `sent`, logs the
contradiction, and does not re-route or bounce. A receipt for another session or destination
does not contradict this attempt. The exact session, destination, and count match still applies.

**Why the session id, and not a timestamp.** The sender has to know whether the relay's
answer covers the whole life of its journal entry. A timestamp comparison would put a
correctness decision on another machine's clock, which D5 forbids. The session id makes the
test exact and clock-free: the sender records the `relaySessionId` under which it durably
committed the current attempt's socket enqueue, and the proof counts only when the two ids match
(§9.2). A **link flap** keeps the id and keeps the proof; a **relay restart** changes it and
the sender has no proof at all — which since §25's B37 means the entry simply waits out
`forwardTimeoutMs` and is recorded lost (§9.3).

**The failure direction is deliberate.** Every ambiguity resolves toward no attempt proof and
no change to the entry or its scheduler. The legacy migration-wide value alone is not enough
(amended — §31, B46). Doing nothing costs at worst one organism. Re-routing on bad proof costs
a duplicated one. Risk 2 names the implementation that gets this wrong: one that treats
**silence** as proof. Silence is never proof in this contract. Only a current relay statement
or a current peer NACK can authorize progress.

**And the direction got cheaper to hold to, not more expensive** (added — §25, B37). Before
that set, refusing a doubtful proof cost a 24-hour wait and then an automatic bounce that could
duplicate the organism after all — the one exception at-most-once carried. It now costs a
record marked `lost`. **The safe answer no longer has a duplication case hiding behind it.**

---

## 6. Message catalogue

Twelve types. **None is new in M4** — the non-delivery proof rides on the existing
`MIGRATION_NACK`, and the operator commands are deliberately not wire operations (§7.5).

**Thirteen since `contract-b/4.0`, and the thirteenth is the only message this project has
added since M3** (amended — §22, B26): `FORWARD_RECEIPT` (§6.12). The operator commands stay
off the wire in shape — B28 authenticates the **path** to them and does not turn them into
messages — so the catalogue grows by exactly one.

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
| `FORWARD_RECEIPT` | relay → sender (added — §22, B26); **not** copied to subscribers | nothing |
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
| `peerId` | string | yes | Stable identity of this client. `1`–`64` characters, `[A-Za-z0-9._-]`. It is what makes a slot reclaim work across a restart, so it **MUST** be persisted (§7.4). **It is now also authenticated** (amended — §22, B22): the relay **MUST** refuse the connection when this value disagrees with the `peerId` the credential named, and the peer whose id was borrowed observes nothing (§3.1). |
| `role` | string enum | yes | `"peer"` — owns a world and a slot — or `"archive"` — a read-only subscriber (§5.1). **The credential's grant decides which values are legal for this connection** (amended — §22, B27), and a role the grant does not carry is refused with `4003`. |
| `protocolVersion` | string | yes | `"contract-b/4.2"` (amended — §31, B46; `"contract-b/4.1"` before it, §25 B37; `"contract-b/4.0"` before that, §22 B32). A different **major** closes with `4000`. **A value below the relay's published `minContractVersion` closes with `4003`** (added — §22, B25) — a compatibility refusal and never a security one, because this string is chosen by the peer that sends it. |
| `gameVersion` | string | yes | The game version behind this sidecar, from the mod's `CONFIG_UPDATE`. Empty while no mod is connected, and always empty for an archive. |
| `sidecarVersion` | string | yes | Informational. The archive sends its own version here. |
| `simulationSize` | float | no | `S`, when a mod has already reported one. |

A second connection **that authenticated as** a `peerId` which is already live **MUST** cause
the relay to close the older connection with `4006` and serve the newer one — the same
self-healing rule `contract-a.md` §2 gives the mod socket (amended — §22, B22). A connection
that presents somebody else's `peerId` never reaches this rule: it is refused at the
credential check, before the upgrade or at the handshake, and the live peer is not told,
because nothing happened to it (§3.1, §3.2).

**Compatibility enforcement at connect.** The relay **MUST** refuse a peer whose
`gameVersion` is incompatible with the map's, and it **MUST** be loud about it: it closes
with `4003`, logs one error naming both versions, and reports the refusal in the next
`PEER_STATUS` as `lastRefusal` on that slot. A silent version mismatch is indistinguishable
from a dead peer — under M4 both end with a lane routed around them — and M4 crosses two
independently updated installs, so this is the failure most likely to waste an evening. An
**empty** `gameVersion` is not a mismatch: it means no mod is connected yet.

**This game-version refusal is kept deliberately under D22, and §22 does not touch it**
(added — §22, B31). D22 makes the **contract** version the map's membership test and the game
version a per-machine matter answered by a published support matrix — which reads like a reason
to retire this paragraph, and the owner decided on 2026-08-11 not to: *"lets not change this
then we will reconsider in the future if there's issues i think we can leave it like its working
now."* So the rule above stands exactly as written, as the fourth of the **four kept exceptions**
B31 names to the game version's diagnostic-only rule. What that costs an operator, and what a
version-skewed map looks like, is stated in B31 where an operator will read it.

**The contract-version gate is the rule D22 actually adds, and it sits beside the one above**
(added — §22, B25). The relay **MUST** refuse a peer whose `protocolVersion` names a different
**major**, with `4000` (unchanged, §3.2), and **MUST** refuse one below its published
`minContractVersion`, with `4003`, a log line naming both versions and the same `lastRefusal`
on that slot. The floor's value is published in `HANDSHAKE_ACK` and on `PEER_STATUS` (§6.2,
§6.5) so a peer can read what it failed before it is refused again, and it defaults to **no
minimum**, which is the only honest default for a map whose operator has not decided one.

**Say both halves of what that gate is, in the same breath, or an implementer will assume the
first implies the second** (added — §22, B25):

| The gate **is** | The gate is **not** |
|---|---|
| A **compatibility** control. It keeps an honestly stale peer off a map it would degrade — and `dev_environment.md`'s *The minors* is the episode that earns it: a pre-`3.3` sidecar answered an upgraded neighbour's `W` exports with a **permanent** `MALFORMED_MESSAGE`, so two lanes ran at ~40 hops/min against ~4–6 everywhere else and two other slots pinned at `inboundQueueMax`. Nothing was lost; the map was simply not operationally complete until every peer upgraded. | A **security** control. `protocolVersion` is attacker-chosen text, exactly as §13 item 7 says `contractAVersion` is. A peer that edits one string walks through this gate. It stops the honest and inconveniences nobody else. |
| A statement about **this map**, set by its operator, raised only **after** the release that satisfies it exists (D25 — GitHub Releases pushes nothing, so publication is the whole fleet-moving mechanism). | A version **negotiation**. Nothing is negotiated: the peer states, the relay admits or refuses, and §4's compatibility rule between peers is untouched. |
| The reason the relay is the right place for it: **the relay is the only party that sees every peer's claimed version at once**, and the peer that suffers from staleness is the stale peer's *neighbour*. | A substitute for the support matrix. The matrix answers *which build runs on my game*; this gate answers *may this build join this map*. Two layers, two tests, and they never meet (D22). |

```json
{
  "protocol": "contract-b/4.2",
  "type": "HANDSHAKE",
  "messageId": "9d1a4b77-2c60-4c1e-9f03-77a1c8e4b510",
  "sentAt": 1785693597011,
  "data": {
    "peerId": "peer-lan-slot5",
    "role": "peer",
    "protocolVersion": "contract-b/4.2",
    "gameVersion": "0.6.3.1",
    "sidecarVersion": "0.4.0",
    "simulationSize": 2000.0
  }
}
```

**The credential is not in that frame and never will be** (added — §22, B22). It is on the
HTTP upgrade that carried it, which is the only place §3.1 permits:

```http
GET /contract-b/v4 HTTP/1.1
Host: relay.example.net
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: x3JJHMbDL1EzLkh9GBhXDw==
Authorization: Bearer peer-lan-slot5.9f3c1a2b7e4d05688c1f0a37d5b9e264
```

The relay splits that credential on its **last** `.`, verifies the secret against the
verifier it holds for `peer-lan-slot5`, and then holds the name until the first frame
arrives. The frame above says `"peerId": "peer-lan-slot5"`, the two agree, and the session
proceeds.

**The refusal that is the whole security property.** The same credential, presented by
somebody who read `peer-main-slot1` off the public status page and wants its slot:

```http
GET /contract-b/v4 HTTP/1.1
Host: relay.example.net
Authorization: Bearer peer-lan-slot5.9f3c1a2b7e4d05688c1f0a37d5b9e264
```
```json
{
  "protocol": "contract-b/4.2",
  "type": "HANDSHAKE",
  "messageId": "5e2b90c1-77af-4d13-b0a6-31c8e5079f42",
  "sentAt": 1785693597100,
  "data": {
    "peerId": "peer-main-slot1",
    "role": "peer",
    "protocolVersion": "contract-b/4.2",
    "gameVersion": "0.6.3.1",
    "sidecarVersion": "0.4.0"
  }
}
```

The upgrade succeeded — the credential is valid — and the **handshake** does not: the
credential names `peer-lan-slot5` and the frame claims `peer-main-slot1`.

```
← close 4003 "peerId does not match the authenticated credential"
```

and the relay logs one error. **What does not happen is the whole test** (Risk 1): slot 1 is
not closed with `4006`, slot 1's connection is not touched, `PEER_STATUS` does not change, no
`lastRefusal` appears on slot 1, and `peer-main-slot1` — which did nothing and is at fault for
nothing — **observes nothing at all**. Under M4's shared token this same sequence took slot 1
off the map in one frame.

**A peer refused for its contract version, and what it can read afterwards** (added — §22,
B25). A `contract-b/4.0` sidecar dialling a relay whose operator has published a floor of
`contract-b/4.2`:

```
← close 4003 "protocolVersion contract-b/4.0 is below this relay's minimum contract-b/4.2"
```

and, in the next `PEER_STATUS` every other client receives, on that peer's slot:

```json
{ "slot": 5, "position": { "col": 1, "row": 1 }, "peerId": "peer-lan-slot5",
  "live": false, "modConnected": false, "gameVersion": "0.6.3.1",
  "simulationSize": 2000.0, "exportEdges": ["E", "N"],
  "darkSinceMs": 1785693719004,
  "lastRefusal": "contract_version_below_minimum: contract-b/4.0 < contract-b/4.2" }
```

That is what stops a stale peer from reading as a dead one, which is the same reason §6.1's
game-version refusal has always written `lastRefusal` (§6.5). **The remedy names who must
act**: the peer's own operator upgrades from the published release, and nobody on the relay's
side can do it for them (D25).

### 6.2 `HANDSHAKE_ACK` — relay → client

| Field | Type | Required | Semantics |
|---|---|---|---|
| `relayVersion` | string | yes | Informational. |
| `protocolVersion` | string | yes | `"contract-b/4.2"` (amended — §31, B46; `"contract-b/4.1"` before it, §25 B37; `"contract-b/4.0"` before that, §22 B32). |
| `minContractVersion` | string | no | **The floor this relay admits** (added — §22, B25). Absent means **no minimum**, which is the default. A peer that reads it can say what it will need before it needs it, and an operator surface can say which peers are one release from being refused. It is a **compatibility** statement and never a security one (§6.1). |
| `limits` | object | yes | **The published capacity table** (added — §22, B24), as §3.3 defines it: the limits this relay is actually running with, not the shipped defaults. A peer reads it at connect and **MUST** respect it; a peer that cannot is a peer that will be shed with `4007`. It is the relay's own configuration, so it is authoritative and it changes only when the relay restarts. |
| `relaySessionId` | `uuid` | yes | **New in M4.** Minted once at relay start, constant for the life of the relay process. It is the scope of the forwarding record (§5.2), and a sidecar **MUST** persist it against every journal entry it hands over while this connection is live (§9.2). |
| `assignedSlot` | number (int) | no | The slot this `peerId` already holds, when the relay remembers one. Absent for a first-time peer and always absent for an archive. |
| `assignedPosition` | object `{col,row}` | no | Its position. Present exactly when `assignedSlot` is. |
| `map` | object `{width,height}` | yes | The rectangle right now. `{"width":0,"height":0}` before the first placement. |
| `slotCount` | number (int) | yes | How many slots are reserved right now. `0` before the first placement. Renamed from M3's `ringSize`. |
| `receivedAt` | `timestampMs` | yes | The relay's own clock. Informational, and the anchor the archive uses to order records written by six machines' clocks. |

```json
{
  "protocol": "contract-b/4.2",
  "type": "HANDSHAKE_ACK",
  "messageId": "0b4e2a13-5d77-4b90-8a21-6f0c19d4e772",
  "sentAt": 1785693597019,
  "data": {
    "relayVersion": "0.4.0",
    "protocolVersion": "contract-b/4.2",
    "relaySessionId": "5f0b9c31-77ad-4e26-9a4c-1b83d206ef95",
    "assignedSlot": 5,
    "assignedPosition": { "col": 1, "row": 1 },
    "map": { "width": 3, "height": 2 },
    "slotCount": 6,
    "receivedAt": 1785693597018,
    "minContractVersion": "contract-b/4.0",
    "limits": {
      "maxConnectionsPerPeer": 2,
      "maxConnectionsPerAddress": 8,
      "maxFramesPerSecond": 50,
      "maxFrameBytes": 8388608,
      "maxBytesPerSecond": 4194304,
      "maxClaimsPerMinute": 12,
      "maxGenomeRequestsPerMinute": 30,
      "maxSubscribers": 4
    }
  }
}
```

**`limits` is the first thing on this wire the relay tells a peer about the relay**
(added — §22, B24), and it is here rather than on a later frame because a peer that learns its
ceilings after it has already exceeded them learns them from a `4007`. The values shown are
§3.3's defaults; a relay that was configured differently publishes what it is running with,
never the table (D20).

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
  "protocol": "contract-b/4.2",
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
      "lostForwardTotal": 0,
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
  "protocol": "contract-b/4.2",
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
    "stats": { "population": 0, "custodyDepth": 0, "pacedDepth": 0, "lostForwardTotal": 0,
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
| `lostForwardTotal` | number (int) | no | **What migration costs on this map** (added — §25, B37; amended — §31, B46). Cumulative count of organisms whose current or most recent committed attempt heard no answer within `forwardTimeoutMs` (§9.3). The socket enqueue can fail after the commit, so the count includes conservative local-send losses. A monotonic counter, reset only by losing the journal. **A loss is a fact the operator reads, not a silent repair.** |
| ~~`heldDepth`~~ | — | — | **RETIRED — §25, B37.** It counted outbound entries in the `held` state, and there is no such state. A `contract-b/4.1` sidecar does not send it; a peer that still does is older than that set, and a reader renders it as unknown or ignores it. **The name is reserved and MUST NOT be reused.** |
| ~~`bouncedTimeoutTotal`~~ | — | — | **RETIRED — §25, B37.** It counted entries a hold timeout bounced home, and nothing bounces on a timeout. Same rules as the row above; **the name is reserved and MUST NOT be reused.** |
| `simulatedTime` | float | no | The world's simulated seconds, from the last `HEARTBEAT`. It is what makes a paced rate interpretable. |
| `timeScale` | float | no | **How fast the world is running** (added — §18, B16), copied from the last `HEARTBEAT.timeScale` (`contract-a.md` §5.2) and **never computed here**. `5` is five simulated seconds per real second; `0` is a world standing still, which is a **reading and not a gap** — a reader renders it as such and does not confuse it with absence. Absent means **unknown**: no mod is connected, or no heartbeat has arrived on this session yet. It is the interpretive key to `simulatedTime` and to the two fields below: both advance and are spent in the world's own time, not the wall's. |
| `inboundRatePerSimMinute` | float | no | **The delivery rate limit this sidecar is configured with** (added — §18, B16), from `contract-a.md` §7.5 and its `--inbound-rate` knob (§18, A40). A **setting**, not a measurement, and the sidecar always knows its own — so absence here means the peer's build predates this field, never that it has no limit. **A reader MUST render an absent value as unknown and MUST NOT substitute the shipped default**, which has been changed three times (2.0 → 12.0 → 100.0 on 2026-08-07 alone). It is what makes `pacedDepth` readable: twelve entries queued behind 120 a simulated minute is a blink, and twelve behind 2.0 is six minutes. |
| `inboundRateBurst` | float | no | The token-bucket capacity behind that rate (added — §18, B16), same source and same rules. It bounds the largest clump the pacer can release at once, so a `pacedDepth` that sits below it is a queue that has never actually been paced. |
| `targetTimeScale` | float | no | The speed the game is requesting, distinct from the applied `timeScale`. It is copied from Contract A and is unknown when an older mod omits it. |
| `admissionMode` | string | no | Receiver population policy: `off`, `fixed`, `adaptive-shadow`, or `adaptive`. |
| `admissionTargetTimeScale` | float | no | Achieved speed the receiver's adaptive estimate is sized for. |
| `admissionPopulationLimit` | number (int) | no | Current effective population limit. Absent while an adaptive controller is still learning. |
| `admissionEstimatedLimit` | number (int) | no | Current raw robust estimate, before the controller's rate-limited movement. |
| `admissionCommitted` | number (int) | no | Last reported living population plus accepted arrivals not yet reconciled by a later heartbeat. |
| `admissionClosed` | bool | no | Whether committed load is on the closed side of the limit, including in shadow mode. |
| `admissionEnforcing` | bool | no | `false` distinguishes an observable shadow decision from a receiver that will send `OVERLOADED`. |
| `admissionSampleCount` | number (int) | no | Valid independent capacity samples currently inside the estimator's one-hour window. |
| `admissionRejectedTotal` | number (int) | no | Monotonic count of new migrations refused by the population gate in this sidecar process. |
| `lastSave` | object | no | The mod's save receipt, copied verbatim from `HEARTBEAT.lastSave` (`contract-a.md` §5.2, §15 A21): `atMs`, `simulatedTime`, `population`, and the optional `name`, `bytes`, `durationMs`. |
| `species` | array of object | no | **The world's active species census** (added — §16, B11), copied **verbatim** from the last `HEARTBEAT.species` the sidecar received (`contract-a.md` §5.2, §17 A35). One entry per species with at least one living member or egg, sorted by `bibites + eggs` descending, at most `speciesCensusMax` (32) entries. Absent means **unknown** — no mod is connected, the mod predates `contract-a/2.2`, or no heartbeat has carried one. A present `[]` means a reporting mod with nothing alive in its world, which is a different fact (§10.1). |
| `species[].genericName` | string | yes | The genus half, **raw**: the bytes the origin world's `Species` record holds. Valid UTF-8, 1 to 64 UTF-8 bytes. Leading, trailing and doubled internal whitespace are **legal here**, and no party on this wire may trim, collapse, case-fold, normalize or re-case one. **This is deliberately not the rule `MIGRATION_PAYLOAD.species` carries** (§6.6, §15 B9): that name is a matching key the exporting mod normalizes at the source (`contract-a.md` §16, A34); this one is a display label that must read as the owning world's player sees it (`contract-a.md` §17, A36). |
| `species[].specificName` | string | yes | The specific half. Same rules. The world's display name is `genericName + " " + specificName`. |
| `species[].bibites` | number (int) | yes | Living members of that species in that world, `≥ 0`. Excludes eggs, so it is on the same footing as `population`. |
| `species[].eggs` | number (int) | yes | Unhatched eggs of that species, `≥ 0`. `bibites + eggs` is the game's own `Species.count`. |
| `modVersion` | string | no | **The plugin version behind this peer** (added — §19, B18), copied from `CONFIG_UPDATE.modVersion` (`contract-a.md` §5.1). The sidecar has held it since M2 and has never published it. Absent means **unknown** — no mod is connected, or the peer's build predates this field. It is not a capability statement and a reader **MUST NOT** infer one from it: what a mod can do is stated by `contractAVersion` and, field by field, by presence (`contract-a.md` §3.1). |
| `contractAVersion` | string | no | **The protocol identifier the mod session is speaking** (added — §19, B18), for example `"contract-a/2.3"` — the `protocol` field of that mod's frames (`contract-a.md` §3). Absent means **unknown**, same two causes. It is what turns a missing census or a missing settings block from a puzzle into a fact: a slot reporting `contract-a/2.2` has no settings because its mod cannot send any. |
| `migrationExclude` | array of string | no | **The species that peer's world never exports** (added — §19, B18), copied **verbatim** from `CONFIG_UPDATE.migrationExclude` (`contract-a.md` §5.1, §19 A42). Full names, both halves joined by one U+0020, in the **A34-normalized** form — trimmed, internal runs collapsed — because this is the **matching** lane and these are the exact strings the origin mod compares against. **That is deliberately not the rule `species[]` above carries**: a census name is a display label and travels raw (§16, B11). No party on this wire may normalize, re-normalize, sort, deduplicate or repair an entry. Absent means **unknown**; a present `[]` means the origin mod has the policy **off**. |
| `saveMinutes` | float | no | **Wall-clock minutes between that world's periodic saves** (added — §19, B18), from `CONFIG_UPDATE.saveMinutes` (`contract-a.md` §19, A42). **`0` is a reading, not a gap** — the save timer is off — exactly as `timeScale: 0` is a stopped world, and a reader that folds the two together loses the one fact that explains an absent `lastSave`. Absent means unknown, and a reader **MUST NOT** substitute the shipped default. |
| `saveKeep` | number (int) | no | Rotated saves that world keeps beside the live one (added — §19, B18), same source and same rules. |
| `saveOnQuit` | bool | no | Whether that world saves when its game quits (added — §19, B18), same source and same rules. With the two above it is the whole answer to "what happens to this world if its machine stops". |
| `worldWrapping` | bool | no | **D10's containment fact for that world** (added — §19, B18), from `CONFIG_UPDATE.worldWrapping` (`contract-a.md` §19, A42) — reported by the mod, never written by it. `false` is a reading and a loud one: it names a world that is not containing its own organisms. Absent means unknown. |
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
compatibility habit, not a new behaviour. **It has now paid for itself twice** (added — §19,
B18): §18's three pacing settings were the field after the census, and §19's seven settings are
the field after those. A relay that stores the block as bytes carried both without a line
changing.

**Nothing on this block is confidential, and §22 makes that a rule rather than a circumstance**
(added — §22, B27). Every field here is republished to every peer and to every authorised
subscriber, and B27 states the boundary from the subscriber's side: a public map's
`PEER_STATUS` carries each world's census, mod version, save policy and exclusion list, which
together are a fairly complete profile of a stranger's machine. **A sidecar that must not
publish a value MUST NOT put it on this block**, because no rule downstream will keep it back.

**The relay's own limits are NOT on this block, and the reason is this block's discipline**
(added — §22, B24). D20's rule — *every knob a peer's behaviour depends on must be published* —
is satisfied by §3.3's table riding `HANDSHAKE_ACK` and `PEER_STATUS` (§6.2, §6.5), **not** by
a `limits` key inside `stats`. This block is **peer-authored end to end**: the sidecar copies
it from its mod or reports its own configuration, the relay stores it as the bytes it arrived
as and interprets nothing (below), and a relay-authored field inside it would be the first
value on this block the peer did not write — which is exactly the habit §16's B11 asked the
relay *not* to acquire. The property D20 actually wants is that **a depth is readable against
the cap it is queued behind**, and it holds either way: the page renders each peer's
`pacedDepth` against the peer's own `inboundRatePerSimMinute` (§18, B16) and each peer's frame
and claim behaviour against the relay's published `limits`, both from the same broadcast
(§10.1).

**The settings on this block are read-only, and that is a rule rather than a description**
(added — §19, B18). `modVersion`, `contractAVersion`, `migrationExclude`, `saveMinutes`,
`saveKeep`, `saveOnQuit` and `worldWrapping` are what the **origin mod** reports about itself.
No sidecar, relay, archive or page may act on one: not route, not filter, not schedule, not
refuse, not fill in a default, and above all **not send one back**. There is no settings-write
path in this system, and `contract-a.md` §19, A43 states why a control surface would be a
separate design rather than a reversal of these fields.

**The census is what bounds this block's size, and the cap is why** (added — §16, B11). A full
32-entry census is about 3 KB; a typical rig world reports four to a dozen species and under
1 KB. A stats-bearing `PING` carries one, at `statsIntervalMs` (5 s), and a `PEER_STATUS`
carries one **per slot** — so a six-slot map broadcasts at most ~20 KB every
`statsBroadcastIntervalMs`, and even a 32-slot map broadcasting full censuses stays two orders
of magnitude inside `maxFrameBytes`. `speciesCensusMax` (`contract-a.md` §10) is the constant that
makes that arithmetic hold, and no party on this wire may raise it unilaterally. **The settings
change none of that arithmetic** (added — §19, B18): two short version strings, three numbers, a
boolean and an exclusion list that holds one entry on the shipped default — well under 200 bytes
beside a census measured in kilobytes, and every one of them constant for the life of a mod
session.

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
| `reason` | string enum | yes | `"granted"` (a new slot was placed), `"reclaimed"` (the reservation for this `peerId` was still held), `"updated"` (a repeat claim), `"repositioned"` (same slot, new coordinate, because the map grew), `"handover"` (this peer inherited a slot by operator command — §7.5), `"role_has_no_slot"`, `"protocol_mismatch"`, `"version_incompatible"`, **`"rate_limited"`** (added — §22, B24: this peer is over `maxClaimsPerMinute`, §3.3 — the claim is refused and the connection is **not** closed, because a claim storm is usually a peer whose measured time scale is wandering and a refusal it can read beats a close it must recover from). |
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
  "protocol": "contract-b/4.2",
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

**Those are the only two sources, and the second one is the ONLY source for stats**
(amended — §24, B36). A stats block arriving on a `PING` (§6.11) or riding a `SECTOR_CLAIM`
(§6.3) is stored against its peer and **MUST NOT** schedule a broadcast of its own: it changes
what the next frame *says*, never what the map *is*, and the timer above was going to carry it.
The rule is written as a MUST because the failure is invisible from one peer's seat — each
arrival looks like one harmless frame, and it is the **map** that pays, `slotCount` stats blocks
to every peer and every subscriber, once per arrival per peer instead of once per interval.

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
| `slots[].lastRefusal` | string | no | Present when the relay last refused this peer's connection, naming the reason (§6.1). This is how a version mismatch stops looking like a dead peer. **It now carries three refusal axes and names which one fired** (amended — §22, B24, B25): `game_version_incompatible` (§6.1, unchanged and kept — B31), **`contract_version_below_minimum`** (added — B25, and the string carries both versions so the remedy is legible: `"contract_version_below_minimum: contract-b/4.0 < contract-b/4.2"`), and **`capacity`** (added — B24, naming the §3.3 limit that fired). **A credential failure never appears here** (B22): a refused credential reaches no slot, and writing it on the slot whose id was borrowed would tell an innocent peer it had been attacked and give an attacker a confirmation surface. |
| `slots[].stats` | object | no | The last peer stats block from that peer (§6.3.1). Absent when none has arrived. |
| `slots[].statsAsOfMs` | `timestampMs` | no | Relay clock when that block arrived. Present exactly when `stats` is. **A reader MUST use it to age the stats**: a population from a peer that went dark an hour ago is history, not state. |
| `you` | object | yes | `{"slot": int\|null, "position": {col,row}\|null, "neighbours": {"E": int\|null, "N": int\|null}}` for the receiving client. All null for a subscriber. |
| `observers` | number (int) | yes | Connected read-only subscribers. Informational. **Each one is now individually authorised** (amended — §22, B27), so this number is a count of decisions an operator made rather than of sockets that knew a token. |
| `limits` | object | yes | **The relay's published capacity table** (added — §22, B24), the same object `HANDSHAKE_ACK` carries (§6.2, §3.3). It is republished here so that a page fed only by broadcasts can render each peer's behaviour against the ceilings it is measured on, and so that a peer whose relay was reconfigured under it learns the new values without reconnecting. |
| `minContractVersion` | string | no | **The floor this relay admits** (added — §22, B25). Absent means no minimum. It is on this frame so an operator surface can say which peers are one release away from being refused, and so that D25's *publish, then raise the floor* is a sequence a reader can watch rather than a policy they must be told. |

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
  "protocol": "contract-b/4.2",
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
        "stats": { "population": 231, "custodyDepth": 1, "pacedDepth": 0, "lostForwardTotal": 0,
                   "modVersion": "0.6.1", "contractAVersion": "contract-a/2.3",
                   "migrationExclude": ["Basic bibite"], "saveMinutes": 10.0,
                   "saveKeep": 6, "saveOnQuit": true, "worldWrapping": true,
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
        "stats": { "population": 208, "custodyDepth": 0, "pacedDepth": 0, "lostForwardTotal": 0,
                   "species": [ ... 7 entries, elided ... ] },
        "statsAsOfMs": 1785693731641 },
      { "slot": 3, "position": { "col": 2, "row": 0 }, "peerId": "peer-main-slot3",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731639,
        "stats": { "population": 197, "custodyDepth": 0, "pacedDepth": 0, "lostForwardTotal": 0,
                   "species": [ ... 5 entries, elided ... ] },
        "statsAsOfMs": 1785693731639 },
      { "slot": 4, "position": { "col": 0, "row": 1 }, "peerId": "peer-lan-slot4",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731630,
        "stats": { "population": 244, "custodyDepth": 3, "pacedDepth": 11, "lostForwardTotal": 2,
                   "species": [ ... 32 entries, elided ... ], "truncated": true },
        "statsAsOfMs": 1785693731630 },
      { "slot": 5, "position": { "col": 1, "row": 1 }, "peerId": "peer-lan-slot5",
        "live": false, "modConnected": false, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693719004,
        "darkSinceMs": 1785693719004,
        "stats": { "population": 226, "custodyDepth": 0, "pacedDepth": 0, "lostForwardTotal": 0,
                   "species": [ ... 9 entries, elided ... ] },
        "statsAsOfMs": 1785693718991 },
      { "slot": 6, "position": { "col": 2, "row": 1 }, "peerId": "peer-main-slot6",
        "live": true, "modConnected": true, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"], "lastSeenMs": 1785693731647,
        "stats": { "population": 189, "custodyDepth": 0, "pacedDepth": 0, "lostForwardTotal": 0 },
        "statsAsOfMs": 1785693731647 }
    ],
    "you": { "slot": 4, "position": { "col": 0, "row": 1 },
             "neighbours": { "E": 6, "N": 1 } },
    "observers": 1,
    "minContractVersion": "contract-b/4.0",
    "limits": {
      "maxConnectionsPerPeer": 2,
      "maxConnectionsPerAddress": 8,
      "maxFramesPerSecond": 50,
      "maxFrameBytes": 8388608,
      "maxBytesPerSecond": 4194304,
      "maxClaimsPerMinute": 12,
      "maxGenomeRequestsPerMinute": 30,
      "maxSubscribers": 4
    }
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

**Only slot 1 carries settings in that frame, and that is the honest picture of a rig mid-roll**
(added — §19, B18). Its `contractAVersion` says `contract-a/2.3`, which is *why* it has them;
every other slot's mod is older, so its settings render as unknown while its population, census
and depths stay exact. Slot 1's `migrationExclude` is also the explanation for a shape the page
can otherwise only hint at: `Basic bibite` will be in that world's census and on none of its
lanes (§10.1, §17 B14).

### 6.6 `MIGRATION_PAYLOAD` — sidecar → sidecar, forwarded

The Contract C `MigrationEnvelope`, carried in `data`, with the lineage annex (D11).

| Field | Type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The idempotency key (D2). Minted by the origin **mod**, preserved end to end — **including across a re-route** (§9.2). |
| `kind` | string enum | yes | `"bibite"` in M4. Anything else is answered `MIGRATION_NACK` / `KIND_UNSUPPORTED`. |
| `body.version` | string | yes | The game version that serialized the blob. Authoritative over the blob's own `version` key (`contract-a.md` §4.6). **It is diagnostic metadata, and a NEW reader MUST NOT parse it into a capability or refusal decision** (amended — §22, B31) — the rule §13 item 7 puts on `contractAVersion`, applied to the second version axis (D22, refined 2026-08-11). It is carried so a future cross-version incident is diagnosable from the record instead of reconstructed from memory. **Four shipped readers do decide on it and are kept, deliberately**; B31 names all four and states what a skewed map looks like. |
| `body.bb8` | string | yes | The opaque blob, as a JSON **string**, never nested, never base64. Max `maxPayloadBytes` (4 MiB). **The payload stays opaque and cross-version loading is assumed to work** (amended — §22, B31): D4 is unchanged and unextended, no refusal path is designed on the game-version axis, and neither a normalized canonical schema nor a marker-plus-refusal is built until an incident makes one necessary (D22). |
| `lineage` | object | yes | The annex. Always present; `parents` may be empty. |
| `lineage.genomeHash` | string | yes | The migrant's own genome hash, computed by the **source sidecar** from `body.bb8` with `genome-hash.md`. The archive's join key. **The empty string when the migrant's own genome will not hash** — see below. Always present as a key. |
| `lineage.parents` | array of object | yes | `0`–`2` entries, in `genes.parent1` then `genes.parent2` order. `[]` is normal. |
| `lineage.parents[].entityId` | `entityId` | yes | The parent's id, from the migrant's genes. Signed int32, often negative. |
| `lineage.parents[].genomeHash` | string | no | The parent's genome hash. **Absent means a gap** — the parent genome was not available to hash. |
| `lineage.parents[].gapReason` | string enum | no | Present exactly when `genomeHash` is absent: `"parent_gone"` (no blob was shipped — the usual case), `"blob_invalid"` (`bb8-schema` could not hash it), `"blob_dropped_for_size"` (the mod trimmed it to fit the frame). ~~**`"blob_dropped_for_size"` is still unreachable**~~ **All three values are reachable from `contract-a/2.4` onward** — `contract-a.md` §21, A49 adds the `parents[].blobDroppedForSize` flag that tells the two blobless cases apart — see below. |
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
| `reroute` | object | no | **New in M4; amended — §31, B46.** Present exactly when this frame's `destSlot` is not the one the entry first carried (§9.2). The relay reads only `count` to correlate a queue-full refusal. The receiver does not act on it. The other fields remain informational. |
| `reroute.fromSlot` | number (int) | yes | The `destSlot` this entry originally carried. |
| `reroute.count` | number (int) | yes | How many times this entry has been re-routed, starting at 1. It MUST be positive when `reroute` is present. Absence means count 0. It is bounded by `maxReroutes` (§9.2) and is relay-visible only for `refusedAttempt` correlation (amended — §31, B46). |
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

**Contract debt, closed in M5 — the sidecar can now emit all three `gapReason` values**
(closed by `contract-a.md` §21, A49; **no amendment of this document was needed and none is
claimed**, because the enum and every obligation around it are unchanged). Through M3 and M4 a parent
the mod dropped for frame size arrived on Contract A as an `entityId` with no `payload`, which
is byte for byte what a dead parent looks like, so the sidecar recorded `"parent_gone"` and
`"blob_dropped_for_size"` never appeared on this wire. The value stayed in the enum: the mod
logged each drop on its own side, a receiver must already tolerate every value here, and
removing it would have been a wire change for a field a reader must handle defensively anyway.
**Closing the debt needed one additive optional flag on Contract A's `parents[]`, and
`contract-a.md` §21, A49 adds it** — `parents[].blobDroppedForSize` — so from a
`contract-a/2.4` mod onward the sidecar maps a blobless entry carrying the flag to
`"blob_dropped_for_size"` and every other blobless entry to `"parent_gone"`
(`contract-a.md` §14 A12, §12 item 8). **Nothing on this wire changes for it**: the enum, the
field and every receiver's obligation are exactly as they were, and a `contract-a/2.3` mod goes
on producing the two-value behaviour above, indefinitely and conformantly.

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
   | A **tombstone** — this `migrationId` was delivered to the mod, so delivery is proven | `MIGRATION_ACK` immediately, with `duplicate: true`. It bypasses the sidecar's deferred-send class (amended — §28, B43). This can be the first ACK that reaches the relay when the ordinary durable ACK is still pending. |
   | A **journal entry that is not yet a tombstone** — journaled, and still waiting for its mod, whether behind the delivery rate limit or awaiting `MIGRATE_IN_ACK` | **Nothing.** Silence, and one log line. §6.7 forbids an ACK before the receiving mod's `MIGRATE_IN_ACK`, and this is that rule, not an exception to it. |

   **The second row is the one that matters, and pacing is why** (Risk 9). A paced delivery
   can sit in this journal for minutes of simulated time. ACKing it here would release the
   sender's custody at the moment the organism is *least* delivered — journaled at the
   receiver, not yet spawned — and if that receiver then dies, both sides have let go. **The
   sender is not waiting on this answer to act** (amended — §25, B37): since that set it has
   no retry to spend and no clock running against a live destination, so the cost of the
   silence is a delay in a record and never an action. **The rule matters more, not less, for
   the peers that still retry**: every sidecar older than B37 produces exactly this duplicate,
   and this row is what keeps it harmless (§25, B37, *the transition*).

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
  "protocol": "contract-b/4.2",
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
  "protocol": "contract-b/4.2",
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

The one exception is the deduplication path of §6.6 step 1. A tombstone proves that the mod
delivered this `migrationId`. Thus, an immediate re-ACK carries the correct meaning. It can be
the first ACK that reaches the relay. The ordinary durable ACK can still be pending (amended —
§28, B43). **A duplicate that is only journaled is not that case. The sidecar sends nothing.**
§6.6 step 1's table agrees with this section. This section is the authority (§14, B6).

**Pacing does not change this rule, and Risk 9 is the reason to say so out loud.** A paced
delivery can sit in the receiver's journal for minutes of simulated time before the mod sees
it, and the `MIGRATION_ACK` waits that whole time. That silence is not an orphan, and nothing
on the sender's side treats it as one: it does not re-send, and the only deadline it carries is
`forwardTimeoutMs`, which closes a record rather than moving an organism (amended — §25, B37;
§9.2's hold clock was what kept the sender honest before that set removed it). Moving the ACK
earlier — to the
journal write — would fix the silence by breaking the thing the chain exists for: the spawn
is the proof of delivery.

**An ordinary ACK becomes durable before it becomes sendable** (added — §28, B43). After
`MIGRATE_IN_ACK`, the sidecar first records the inbound entry as complete with
`ackedUpstream: false`. That durable state means it still owes the source a `MIGRATION_ACK`.

| Pending-ACK rule | Requirement |
|---|---|
| Shared pacing | Ordinary journal-backed ACKs use the same published frame and byte budgets as outbound `MIGRATION_PAYLOAD` frames (§3.3). |
| Priority | The scheduler offers pending ACKs before it offers new outbound payloads. This releases existing sender custody before this peer creates more outbound custody. |
| Retry | If no relay session exists, pacing defers the ACK, or local enqueue fails before acceptance, `ackedUpstream` stays `false`. The scheduler retries it, including after a sidecar restart. |
| Retention | A completed inbound entry with `ackedUpstream: false` **MUST NOT** expire under `exportRetentionSeconds`. It remains until the ACK reaches the attempted local-write boundary. |
| Completion | After the live relay connection accepts the ACK into its local write queue, the sidecar records `ackedUpstream: true` durably. A later connection loss keeps the existing conservative ambiguity. The source retains its entry until an ACK arrives or §9.3 records a loss. |

**The immediate replies stay outside this deferred class.** A tombstone duplicate re-ACK and
a sidecar-generated `MIGRATION_NACK` answer one relay-paced migration arrival. They send on the
ordinary control path and remain charged to the sidecar's capacity budget. The relay's rate `R`
bounds their aggregate causal rate, so they do not wait behind a journal drain. An opportunistic
duplicate re-ACK does not clear `ackedUpstream`. The ordinary durable retry still does that after
its own local enqueue succeeds.

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
  "protocol": "contract-b/4.2",
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
| `neverForwarded` | bool | no | **New in M4; amended — §31, B46.** Present on proof-bearing relay non-delivery NACKs. It retains its migration-wide 4.1 meaning. `true` means this relay session forwarded no attempt with this `migrationId`. `false` means it forwarded at least one attempt or cannot say. A 4.2 queue refusal can carry `false` with an exact `refusedAttempt` after an earlier accepted attempt. Graceful drain omits the field. |
| `relaySessionId` | `uuid` | no | Present exactly when `neverForwarded` is. The statement applies only to this relay session. Graceful drain omits it. A sender MUST match it to the current journal attempt before using either proof form. |
| `refusedAttempt` | object | no | **New in `contract-b/4.2`** (added — §31, B46). Present only on queue-full `NOT_FORWARDED`. It identifies the payload that queue admission rejected. Older relays omit it. Missing correlation is never proof. |
| `refusedAttempt.destSlot` | number (int) | yes | The rejected payload's `destSlot`. |
| `refusedAttempt.rerouteCount` | number (int) | yes | The rejected payload's `reroute.count`, or 0 when its `reroute` object was absent. |

| Code | Class | Sent by | Cause |
|---|---|---|---|
| `SLOT_VACANT` | **permanent** | relay | `destSlot` names **no reservation at all** — released, handed to nobody, or never issued. Slot numbers are never reused (§2), so this world never returns and no retry can ever succeed. **Reclassified in M4**: it was `transient` in M3, when a vacant slot and an offline peer were the same answer. |
| `PEER_OFFLINE` | transient | relay | **New in M4; amended — §28, B43.** The reservation exists, but its peer has no live connection. This also covers a target connection that stops before atomic queue acceptance. This is M3's `SLOT_VACANT` case, given its own name so the permanent one can mean what it says. |
| `NOT_FORWARDED` | transient | relay | **New in M4; amended — §31, B46.** The relay declined the frame before its attempted-write boundary. A full destination queue includes attempt proof and does not close that peer. Graceful drain includes no proof fields and starts no source-side walk. |
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

**Queue-full `NOT_FORWARDED` has one bounded source action** (amended — §31, B46). The action
requires `neverForwarded` to be present, but its migration-wide value can be true or false.
`refusedAttempt.destSlot`, `refusedAttempt.rerouteCount`, and `relaySessionId` MUST match the
current journal attempt. A receipt for that session and destination contradicts the proof.
Missing or stale correlation, a session mismatch, or a contradictory receipt leaves the entry
and its scheduler unchanged. Graceful-drain `NOT_FORWARDED` has no proof fields and has the
same result.

**A sidecar MUST NOT send `MIGRATION_NACK` after it has durably journaled the payload.**
This is what makes a sidecar NACK a *definitive* statement that custody never moved, and it
is what makes both the bounce-back and the M4 re-route safe. It is the single most
load-bearing sentence in this document, and the one an implementation is most likely to
violate by adding a NACK to a later failure path. See §9.

**A sidecar-generated NACK uses the immediate control path** (added — §28, B43). It does
not wait behind ordinary durable ACKs or outbound payloads. The relay has paced the arrival
that caused it, so §3.3 bounds the aggregate reply rate.

**A relay NACK describes one attempt, but the legacy boolean describes the migration.** That
difference is why 4.2 adds `refusedAttempt` (amended — §31, B46). An earlier destination can
have accepted or peer-refused the migration before a later transport queue refuses it. In that
mixed history, `neverForwarded` is false while the new attempt proof is exact. A missing attempt
object must remain conservative. It does not authorize a re-route.

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
  "protocol": "contract-b/4.2",
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
simply cannot speak for the period before it started, so it says `false` and the sender **does
not re-route** (amended — §25, B37: before that set it *held*, and the entry ran a 24-hour
clock; it now waits out `forwardTimeoutMs` and is recorded lost, §9.3):

```json
{
  "protocol": "contract-b/4.2",
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

A queue-full refusal after an earlier accepted or peer-refused attempt can carry
`neverForwarded: false` and still prove this exact transport attempt:

```json
{
  "protocol": "contract-b/4.2",
  "type": "MIGRATION_NACK",
  "messageId": "32fd8d96-8ac2-4c38-a39e-08d369b8547e",
  "sentAt": 1785693840250,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "sourcePeer": "",
    "destPeer": "peer-lan-slot4",
    "code": "NOT_FORWARDED",
    "class": "transient",
    "message": "slot 7 has a full paced migration transport queue",
    "retryAfterMs": 15000,
    "neverForwarded": false,
    "relaySessionId": "c8177a34-90bd-4f51-8e02-4d6b915ca7e3",
    "refusedAttempt": { "destSlot": 7, "rerouteCount": 2 }
  }
}
```

The sender acts only if session, destination, and count match its current attempt. A graceful
drain uses the same code but omits all three proof fields. It does not consume a destination or
start the first-refusal deadline.

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
  "protocol": "contract-b/4.2",
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
| `body.version` | string | yes | The game version the blob was serialized by. **Diagnostic metadata here too** (amended — §22, B31): it is an input to `genome-hash.md`'s projection, which is an **identity** computation and not a capability one, and no reader may turn it into a refusal on this path. A genome the requester cannot hash under that version is a hash mismatch and is handled as one, below. |
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
  "protocol": "contract-b/4.2",
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
  "protocol": "contract-b/4.2",
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
      "lostForwardTotal": 2,
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

### 6.12 `FORWARD_RECEIPT` — relay → sender (added — §22, B26)

**New in `contract-b/4.0`, and the only message added since M3.** Each accepted
`MIGRATION_PAYLOAD` triggers one receipt attempt. The relay sends the receipt to the **sender**
(§5.2, B26; amended — §28, B43).
It is the cheapest frame on this wire: four fields, no body, no answer, and **no fan-out** —
a subscriber is not copied, because a receipt is a fact about the sender's own journal and not
about the migration.

| Field | Type | Required | Semantics |
|---|---|---|---|
| `migrationId` | `uuid` | yes | The migration that was forwarded. It is the sender's join key into its own journal (`contract-a.md` §7.1). |
| `destSlot` | number (int) | yes | The slot whose destination transport accepted the frame. It is the `destSlot` the sender recorded. The echo lets a sender distinguish two re-routed attempts. |
| `relaySessionId` | `uuid` | yes | The session in force at queue acceptance. **A receipt is a statement about one session and nothing else** (§5.2). This field identifies that session. |
| `forwardedAt` | `timestampMs` | yes | The relay's clock when it creates the receipt after transport-queue acceptance (amended — §28, B43). Informational (D5), and useful only for a log. |

| Rule | Statement |
|---|---|
| When | Immediately after atomic transport-queue acceptance (amended — §28, B43). Receipt enqueue is outside the record-to-queue action. It does not wait for physical writing. A queued frame lost with its connection remains indistinguishable from a delivered frame. |
| One per forward | A **re-route** of the same `migrationId` produces another receipt. A sender that holds two receipts under one `migrationId` has written the frame twice; that is a fact about its own re-routes, never a duplicate organism (the `migrationId` is preserved and the destination deduplicates — §6.6). **A conforming sender since §25's B37 writes a frame once per DESTINATION and never re-forwards**, so a second receipt names a second destination — and a sender older than that set may still produce several under one destination, which this rule has always covered. |
| What the sender does with it | **Records it against the journal entry, durably** (§7.4, §9.2), and nothing else. It sends no answer. A receipt changes no handoff state on its own: an entry that is `sent` stays `sent`. |
| Not delivery | It is **not** `MIGRATION_ACK`. Custody moves when the destination sidecar journals and its mod acknowledges (§9.1). A receipt says the relay accepted a local attempted write. |
| Not proof of non-delivery | And it can never become one. A matched relay proof or peer NACK (§6.8, §9.2) can authorize a re-route. A receipt only makes the safe answer more certain: this session and destination were accepted, so the current attempt stays `sent` (amended — §31, B46). |
| Best effort, and the failure direction is safe | A receipt the sender never sees costs nothing but the certainty it would have added — the entry stays `sent`, which is where the receipt would have kept it anyway. The relay **MUST NOT** delay, block or fail a forward on account of a receipt it could not send. |
| Bounded | A receipt uses the bounded ordinary/control queue, not the destination migration queue (amended — §28, B43). If the sender's ordinary queue is full, the receipt is dropped, not queued indefinitely. See the row above for why that is safe. |

```json
{
  "protocol": "contract-b/4.2",
  "type": "FORWARD_RECEIPT",
  "messageId": "3f2a71b8-4c09-4d6e-9a15-8b0c7e42d391",
  "sentAt": 1785693732104,
  "data": {
    "migrationId": "0f6c8b3e-2c41-4a8f-9d1e-7a3b5c9d0e12",
    "destSlot": 5,
    "relaySessionId": "5f0b9c31-77ad-4e26-9a4c-1b83d206ef95",
    "forwardedAt": 1785693732103
  }
}
```

**What it changes for a sender, in one sentence:** the sender's own journal records that the
current attempt was accepted, with its destination and session. A relay restart cannot take
that fact with it
— **but a relay restart still takes the relay's ability to answer for the *absence* of a
forward**, which is the direction that matters for a re-route, and §9.2 is unchanged in every
particular.

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
   `slot = maxSlotEverIssued + 1`. **Usable is narrowed under `contract-b/4.0`: the three
   growth cases below apply only while the rectangle has no hole** (amended — §22, B29). A
   `preferredPosition` that would extend an axis while any hole exists is **ignored** and
   falls through to rule 6, which fills the hole. The hole case itself is unchanged and always
   usable. **Holes before growth is now the map's rule and not only auto-placement's**, because
   under churn a preference is an ordinary stranger's configuration file rather than an
   operator's layout, and one newcomer that extends an axis creates `height` (or `width`)
   positions and fills one of them. Usable means one of:
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

**The join kit still builds the rig's layout under B29's narrowing, and that is the test of
the narrowing** (added — §22, B29). Each of the six sidecars sends its own
`preferredPosition` — `(0,0) (1,0) (2,0) (0,1) (1,1) (2,1)` — and at every moment one of them
asks to *extend*, the rectangle is full: `(0,1)` extends the row of a full 3×1, and `(1,1)`
and `(2,1)` then fill the holes that extension created. No claim in that sequence is refused
by the new rule, so the layout the rig runs is unchanged.

**Placement under churn, which is what a public map does all day** (added — §22, B29). M4
placed six known peers and spliced one newcomer, by hand, once each; §13 item 3 named the
three things continuous joining and leaving would stress first, and this is the rule for them.

| Question churn asks | The answer, and why |
|---|---|
| **A peer left. What happens to its position?** | Nothing, until an operator acts. The reservation never expires (rule 1) and a returning peer lands where it was, which is the whole of *return needs no insertion*. **A position becomes fillable only through `--release-slot`** (§7.5), and then it becomes an ordinary hole that rule 6 fills before any axis grows. |
| **And to its address?** | **It is retired forever.** `maxSlotEverIssued` never decreases and a slot number is never reused (D8, D12). This is custody, not tidiness: `SLOT_VACANT` is a **permanent** answer and therefore a valid proof of non-delivery (§6.8), and reissuing an address would silently convert that proof into a lie. **The split is the point** — D12 and D13 separate the address from the coordinate exactly so that *positions* can be recycled while *addresses* never are. |
| **Does the address space grow without bound, then?** | Yes, monotonically, and that is accepted. `maxSlotEverIssued` has **never been tested at any scale** and §13 item 3 says so; WP5 tests it against a synthetic churn harness before strangers are involved (`m5_considerations.md`, DQ3). What the contract fixes is the rule; what a rig measures is the number. |
| **Holes or growth?** | **Holes, always, and by both routes now** (rule 4 as narrowed, rule 6 as written). A map that extended on every join would grow a row of holes per join and route-around would walk all of them. |
| **Which axis extends?** | The **shorter** one, so the rectangle stays close to square. The reason is mixing, not aesthetics: the cycle length on each axis is what genetic mixing depends on, and a map stretched along one axis is a map whose other axis has nothing to route around (§2.1). |
| **How many joins and departures collapse into one broadcast?** | As many as land inside the coalescing window, and the window **widens under churn** — see below. |

**The broadcast bound churn needs is stated with *No storm* below**, because that is where the
coalescing window already lives (added — §22, B29).

**One consequence of churn belongs in the contract rather than in a surprise** (added — §22,
B29). An organism routed around a bypassed slot never sees that world (§8), so a map with
continuous churn is a map with a **continuously shifting cycle**. That is not a defect — D12
chose route-around over a closed lane deliberately — but it means the genetic mixing a public
map produces is **not** the mixing a stable map of the same size produces. The archive records
what actually crossed (§10), so whoever reads the record later can tell which map they are
looking at; nobody can reconstruct it afterwards if the difference is not written down now.

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

**Two rules join it for a public map, and both are about the broadcast *rate* rather than its
spacing** (amended — §22, B29). A broadcast costs `slotCount` stats blocks, so both terms grow
with the map and a fixed floor stops being a bound:

1. **The window widens under sustained churn.** When more than `statusChurnBurstThreshold` (8)
   registry changes land inside one window, the relay **MUST** double the window, up to
   `statusCoalesceMaxMs` (2 000 ms), and narrow it back one step after a window that saw
   fewer. Every rule above still binds — the last frame of a burst is always sent — and the
   ceiling is arithmetic a reader can check: `60 000 / statusCoalesceMs` broadcasts a minute at
   the floor, `60 000 / statusCoalesceMaxMs` under a storm.
2. **A repeat claim that changes nothing structural broadcasts nothing.** A `SECTOR_CLAIM`
   answered `reason: "updated"` whose `slot`, `position`, `exportEdges`, `borderEdges`,
   `modConnected`, `gameVersion` and `simulationSize` are all unchanged **MUST NOT** raise the
   epoch or trigger a broadcast. Its stats ride the next `statsBroadcastIntervalMs` timer,
   which was going to send them anyway (§6.5, §14 B4). **This is the epoch rate's actual
   cause, measured**: the living deployment's slot 6 issued **64** placement claims in one day
   against two or three from each local slot, every one a re-claim with `reason: "updated"` as
   its measured time scale wandered (`m5_considerations.md`, DQ3). Not a reconnect, not a
   defect — 64 epochs, on a six-slot map, from one peer, for nothing. The relay still answers
   every claim with a `SECTOR_GRANT`, because the claimant is owed an answer; what it stops
   doing is telling everybody else.

**And a third, which is the same rule for the other half of the door**
(added — §24, B36). Rule 2 above told an implementer that a stats block riding a `SECTOR_CLAIM`
schedules nothing, and said nothing about the same block riding a `PING` — which is where §6.11
actually puts it, once per `statsIntervalMs` per peer.

3. **A stats arrival broadcasts nothing, whichever frame it rode in on.** A stats block on a
   `PING` (§6.11) or on a `SECTOR_CLAIM` (§6.3) is stored against its peer and **MUST NOT**
   raise the epoch or schedule a broadcast. `statsBroadcastIntervalMs` is the only publisher it
   reaches (§6.5, §14 B4). **This one was measured too, and it dwarfs DQ3's 64 epochs**: seven
   peers PINGing once per interval each, unaligned, made the hosted relay broadcast the whole
   map **1.32 times a second** — about four times the intended rate and ~11.6% of the
   deployment's entire transfer bill, for state that had not changed. Rules 1 and 2 bound how a
   *busy* map broadcasts; this one is what stops a *quiet* one broadcasting at all.

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
| relay | — (memory) | The forwarding record, `relaySessionId`, destination transport queues, and identity pace (§3.3, §5.2; amended — §28, B43) | A process exit can lose accepted queued frames. Their attempted-forward records also end with that relay session. A new process can describe only its new session. Its `neverForwarded: true` does not prove an attempt from the old session, because the sender compares `relaySessionId` (§6.8, §9.2). **Deliberately not durable.** |
| sidecar | `<data-dir>/peer-id` | One line, the `peerId` | The peer becomes a stranger and takes a second slot, stranding its old one. Generated once, on first start, if absent. |
| sidecar | `<data-dir>/slot` | One line, the granted slot number | Only the `preferredSlot` hint; rule 1 recovers the slot from `peerId` anyway. |
| sidecar | `<data-dir>/position` | One line, `col,row` | Only the `preferredPosition` hint, and only useful to a peer that lost its `peerId` too. Written on every granted `SECTOR_GRANT`. |
| sidecar | journal | Custody, tombstones, the recorded `destSlot`, **the handoff state, the current committed attempt's `relaySessionId` and `sentAt`, the re-route count, the tried/refused slot set, the first proven-refusal deadline, and `ackedUpstream` for completed inbound entries** (§6.7, §9.2; amended — §31, B46) | Organisms and pending upstream ACKs. This is the one file whose loss is not recoverable, and D2 accepts that as loss. |

The relay writes `ring.json` **before** it answers a `SECTOR_CLAIM` that created or changed a
reservation — an answered grant that is not on disk can hand the same slot to two peers across
a restart. The sidecar writes `<data-dir>/slot` and `<data-dir>/position` on every granted
`SECTOR_GRANT` and reads them once at startup; an explicit `--slot` or `--position` flag
overrides them, and an unreadable value is treated as absent. Neither file belongs in the
journal: losing them costs a placement mix-up, never an organism.

**The journal's new fields are load-bearing and must survive a restart.** §9.2 depends on
every one of them: the handoff state says whether custody may have moved, the `relaySessionId`
scopes the proof, `sentAt` is when `forwardTimeoutMs` starts running (amended — §25, B37: *the
accrued hold time is what a bounded hold counts*), and the re-route count bounds the re-route.
Since B46, the refused-slot set prevents a restart from trying the same transport queue again.
The first proven-refusal deadline bounds the full chain and MUST NOT restart on a re-route,
retry, reconnect, restart, or journal compaction. The sidecar MUST persist the refused
transition, the rejected slot, the tried-slot set, and that deadline in one journal update.
That transition clears the old attempt's `relaySessionId` and `sentAt`. The next durable send
commit writes fresh values for the new attempt. A pending or refused entry has no active loss
deadline. A sidecar that reconstructs any of these values from memory has lost the safety
property, not only the bookkeeping (amended — §31, B46).

**`ackedUpstream` is equally load-bearing for completed inbound entries** (added — §28,
B43). `false` means the source is still waiting for this sidecar's durable ACK. Compaction
MUST preserve it, and tombstone retention MUST NOT purge the entry while it is false (§6.7).

### 7.5 Operator commands — release, handover, and the custody rules around them

The reservation never expires, so the map needs operator escape hatches. There are three, and
~~**none of them is a wire message**: an operator command is a rare, deliberate, physical act
on the machine that owns the data, and giving it a network surface in a milestone with one
shared token is a poor trade (§3.1). M5 brings authentication, and §13 item 5 records that
release and handover are the first commands that will want an authenticated admin path.~~
**Amended — §22, B28.** M5 brought the authentication, and item 5's condition — *"if the map
ever grows past what one operator can restart at will"* — is met by the venue rather than by
the size: the relay is a hosted service (D24) and a restart drops **every** peer's session at
once, so *at startup* had become a price paid by six innocent peers to fix one. B28 adds an
authenticated admin **path**, and it is deliberately **not a wire message**: nothing in the
Contract B catalogue can trigger any of these acts, and D1's dumb relay routes frames and
still does nothing else.

| Command | Where | What it does |
|---|---|---|
| `multiverse-relay --release-slot <n>` | relay, at startup **or over the admin path** (amended — §22, B28) | Removes slot `n` from `ring.json` and leaves its position a **hole**. Surviving slots keep their numbers, their positions and their relative order. The number is not reused: `maxSlotEverIssued` never decreases. **This is the operator's answer for a position that will never be filled again** (added — §22, B29): the position rejoins the map as an ordinary hole that the next newcomer fills before any axis grows, and the address stays retired forever, so every journal entry that names it still gets its permanent `SLOT_VACANT`. |
| `multiverse-relay --handover-slot <n> <newPeerId>` | relay, at startup **or over the admin path** (amended — §22, B28) | **New in M4** (D15). Rebinds the reservation — slot number **and** position — to a different `peerId`. The map does not change shape and no lane moves. **It is also the credential recovery path** (added — §22, B22): a stranger who lost their join string is handed their own slot back under a new `peerId` and a freshly minted credential. There is no other recovery, and §3.1 says so rather than implying it. |
| `multiverse-relay --evict-peer <peerId> [--for <duration>]` | relay, at startup or over the admin path | **New in `contract-b/4.0`** (added — §22, B28). Closes that peer's connection with `4005` and refuses it for the stated period, or until lifted. **It releases nothing**: the reservation, the slot number and the position all survive untouched, so the peer's return is an ordinary `reason: "reclaimed"` and its journal is still addressable throughout. Eviction is a **liveness** act, not a placement act, and the map treats an evicted peer exactly as it treats a dark one (§8). It is what `m5_considerations.md` DQ7 leaves for a peer that will not stop when suppressing its text at the renderer was not enough. |
| `multiverse-sidecar --release-inflight <migrationId> bounce\|drop` | the sidecar that holds the journal | **New in M4** (D2, D12). Resolves one unresolved outbound entry by hand, before `forwardTimeoutMs` records it lost (§9.3). **It stays on the sidecar's own machine and gets no admin path**, because custody is local (D2) and the machine is a stranger's. The receipt B26 adds is what reduces how often anybody needs it (§5.2, §13 item 6). **Since §25's B37 it is the ONLY way left to duplicate an organism on this map** — nothing bounces a forwarded entry automatically any more — and §9.3 requires the command to say so in its own output before it acts. |

**The admin path, and the four properties that make it safe** (added — §22, B28).

| Rule | Statement |
|---|---|
| It is not on this wire | The admin surface is a **separate listener** on the relay, never the Contract B WebSocket. No frame of §6's catalogue can invoke an act, and no peer or subscriber can reach one. A relay that accepted an admin instruction on the peer wire would have to authorize per message on the path that D1 keeps free of decisions. |
| Authentication is §3.1's, with a third grant | The same credential mechanism, an **admin** grant, disjoint from the peer and subscribe grants (§5.1, B27). An admin credential is minted at the relay's console like any other and is never a peer's. The listener binds loopback by default and **MUST** be TLS if it is bound anywhere else (B23). |
| **The act stays deliberate: the report comes first, and it is the same report** | `--release-slot`, `--handover-slot` and `--evict-peer` **MUST** print the full consequence before acting, over the admin path exactly as at a console: the slot, its position, its `peerId`, how long it has been dark, which peers' effective lanes change, and which positions become holes. Over the path this is **two calls**: the first returns the report and a single-use confirmation token bound to the act and the current `ring.json` state; the second performs the act and **MUST** be refused if the map changed underneath the token. A confirmation an operator cannot see is not a confirmation. |
| Every act is audited | One durable line per act: which grant, which act, which slot, the state before, and the reason string the operator supplied. D23 defers the *control surface* to M6 and names an audit trail among the questions it must answer; this path answers that question for its own three acts and claims nothing about the larger one. |

**What the admin path deliberately does not become** (added — §22, B28). It is not the control
surface. It changes the **map's registry** — a reservation, an identity, a peer's admission —
and it touches nothing inside a world: no time scale, no save policy, no exclusion list, no
setting a mod reported. §6.3.1's read-only rule and `contract-a.md` §19's A43 are unchanged —
that document's matching set (**`contract-a.md` §21, A47–A52**) supplies the *authentication* A43 listed first
among the questions a control surface must answer, and answers none of the other four
(`contract-a.md` §21, A47; §12 item 10 there) —
and D23 keeps the surface that would write those in M6 (§13 item 8). Three acts on
the relay's own registry are not the thin end of that wedge; they are the escape hatches §7.5
has always had, reachable without dropping six sessions to use one.

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
both commands to list the unresolved entries that name the slot. The relay **cannot**: journals live
on six other machines and D2 keeps custody local — a relay that could enumerate them would be
a relay that reads journals. The division is therefore:

| Question | Answered by |
|---|---|
| What happens to the map? | The relay command's own pre-act report. |
| Who has already LOST organisms addressed to this slot? | `PEER_STATUS.slots[].stats.lostForwardTotal` (§6.3.1) — visible on the status page and in `ringstat` for every peer at once (amended — §25, B37: this row read `stats.heldDepth` before that set, and there is no held depth). |
| **Which** entries are unresolved, and what are they? | `multiverse-sidecar --list-inflight [--dest-slot <n>]`, on the machine that holds the journal. It always prints each entry's `migrationId`, `entityId`, `destSlot`, and handoff state. A `sent` entry also prints its send time and remaining loss time. Pending and refused entries print neither. Since 4.2, the durable first-refusal deadline bounds a queue-refusal walk, but this command does not print that deadline. |

The operator therefore makes one decision with the facts in view; the facts simply come from
two places, because custody does.

**The admin path changes none of that division** (added — §22, B28). The relay still cannot
enumerate journals it is forbidden to read, so the report it returns over the path is the
report it printed at a console — the map's half — and `lostForwardTotal` on `PEER_STATUS` is
where an operator sees who has already lost organisms addressed to the slot (amended — §25,
B37; it read `heldDepth` before that set). What **does** change
with a public map is who can run the third row: `--list-inflight` is typed on the machine that
holds the journal, and after M5 that machine is usually a stranger's. **The operator's honest
answer becomes "ask the peer"**, and the support surface WP7 owes has to say so rather than
implying the relay could look.

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
                  ∧ gameVersion compatible      (with mine)      ← kept gate, §22 B31
                  ∧ simulationSize equal to mine (contract-a.md §13, A10)
                  ∧ slot ≠ me                   (a peer never exports to itself)
```

**The `gameVersion` term is the second of B31's four kept gates, and it is unchanged**
(added — §22, B31). D22 makes the game version a per-machine matter and this walk reads it as
a routing decision, which is the contradiction B31 records rather than resolves: the owner
chose on 2026-08-11 to leave every shipped gate alone. The term stays, `peer_incompatible`
stays as the skip reason below, and **what it produces on a version-skewed map is a partition
rather than a defect** — B31 states the shape and the operator's reading of it.

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

## 9. Custody — the chain, the handoff state, the re-route and the loss

Contract A owns steps 1, 2, 4 and 5 of `system_decomposition.md`'s custody chain. Contract B
owns the middle. **AT-MOST-ONCE CARRIES NO EXCEPTION** (amended — §25, B37).

~~M4 adds one bounded exception to at-most-once and nothing else: an organism forwarded to a
slot that then went dark, with no proof of non-delivery, is held for a configurable timeout —
24 hours by default — and then bounces home by itself. The owner signed that off on 2026-08-05
and accepts the residual duplication risk it names (§9.3).~~ **Superseded — §25, B37**, by the
same owner on 2026-08-17: *"we don't mind much on bibites being lost, we can see migrating as
dangerous, and then there wouldn't be duplicates while making things simpler."*

**MIGRATION IS DANGEROUS, and that is the design rather than a defect in it.** Custody can move
only once. A source may offer another destination only after exact evidence that the current
attempt moved no custody. It never repeats an ambiguous attempt, and it never comes home on an
ambiguous timer. If an accepted attempt does not arrive, it is **lost** — recorded, counted,
readable on the map (§6.3.1, `lostForwardTotal`), and gone.

### 9.1 The chain

```
mod A            sidecar A                relay             sidecar B            mod B
  │ MIGRATE_OUT      │                      │                   │                  │
  ├─────────────────►│ hash + journal+fsync │                   │                  │
  │ MIGRATE_OUT_ACK  │  (handoff: pending)  │                   │                  │
  │◄─────────────────┤                      │                   │                  │
  │  (destroys)      │ journal sent + fsync │                   │                  │
  │                  │ MIGRATION_PAYLOAD    │                   │                  │
  │                  ├─────────────────────►│ queue + record    │                  │
  │                  │                      ├╌╌► archive (copy) │                  │
  │                  │                      ├─ identity-paced ─►│ journal + fsync  │
  │                  │                      │                   │  custody moves   │
  │                  │                      │                   │ MIGRATE_IN       │
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

**The relay queue is not a custody step** (added — §28, B43). Its atomic acceptance is the
attempted-write boundary for the forwarding record. It also triggers the best-effort receipt.
Only sidecar B's durable journal moves custody. A disconnect or process exit can lose a queued
frame. The loss has the same conservative ambiguity as an earlier attempted socket write (§5.2).

The ordinary `MIGRATION_ACK` arrow also uses the sidecar's shared capacity pace. Its durable
pending state survives deferral and restart (§6.7). Immediate NACKs and tombstone re-ACKs use
the bounded control path instead.

**The source commits before it enqueues** (amended — §31, B46). Encoding, connection checks,
and pace admission happen while the entry is still safe to retry. For a bounded refusal chain,
the sidecar checks the absolute first-refusal deadline again after preparation. If preparation
crossed it, the entry bounces before any send commit. Otherwise, the sidecar records
`handoff: sent`, the current `relaySessionId`, and the current `sentAt` durably. Only after that
fsync can it offer the prepared frame to the socket writer. A crash or enqueue error after the commit is an ambiguous
send. The entry stays `sent` and can only receive an answer or become `lost`. This ordering can
lose an organism, but it cannot duplicate one.

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
| `pending` | Journaled, with no ambiguous send. The entry is new, or an exact refusal proved that its last attempt reached no receiver. | **No** |
| `sent` | Durably committed to the current attempt's one socket enqueue, with no terminal answer yet (amended — §31, B46). Carries that attempt's `relaySessionId`, `sentAt`, and any matching `FORWARD_RECEIPT`. The local enqueue can fail after this commit. The sidecar cannot distinguish that failure from an accepted frame after a crash. The receipt does not change the state; it confirms that the state is right. | **Yes — unknowably** |
| ~~`held`~~ | ~~`sent`, and the destination is observed dark. The hold clock accrues here and nowhere else.~~ **REMOVED — §25, B37.** A dark destination is no longer a state change: there was nothing left for the state to authorize once the re-forward and the timeout bounce were gone. A journal written before that set replays a `held` entry as **`sent`**, which is what it always meant. | — |
| `refused` | A statement arrived that proves no custody moved. | **No** |
| `done` | `MIGRATION_ACK` received. Becomes a tombstone. | It moved, and completed. |
| `lost` | `sent`, and no terminal answer arrived within `forwardTimeoutMs` (added — §25, B37). **Terminal.** The entry becomes a tombstone, `stats.lostForwardTotal` increments, and the organism is gone. It is a tombstone rather than a deletion so that a late `MIGRATION_ACK` is still recognised (§9.3). | **Yes — and unknowably forever** |

**B43 adds no handoff state.** After the source commits to its one enqueue attempt, the entry
is `sent`. The relay may then hold the accepted frame in its destination transport
queue. A queue loss never returns that source entry to `pending` and never authorizes a re-route
(§5.2, amended — §28, B43).

**B46 keeps B45's local ordering and narrows its proof.** The source records `sent` before the
socket enqueue. An exact attempt-scoped queue refusal can return it to the safe refusal path.
A local enqueue error cannot do so because it is not a relay proof (amended — §31, B46).

Transitions:

```
journal write ─────────────────────► pending
pending/refused ── durable send commit ───────► sent      (record relaySessionId and sentAt)
sent commit ── one socket enqueue attempt ───► sent      (no second attempt on failure)
sent ── MIGRATION_ACK ─────────────► done
sent ── proof of no custody ───────► refused
sent ── forwardTimeoutMs elapsed ──► lost      (§9.3)
refused / pending ── lane exists ─► re-route, then pending against the new destSlot
refused / pending ── no lane, bounceTimeoutMs ─► bounced home
```

**THERE IS NO ARROW OUT OF `sent` THAT THIS PEER DECIDES** (amended — §25, B37). Two of the
three are answers from somebody else, and the third is a deadline that resolves a **record**
rather than an organism. Nothing re-sends the frame; nothing brings it home. The removed
arrows were `sent → held`, `held → sent`, `held → refused` and `held → bounced home`.

**A bounce is a terminal action, not a state.** The entry leaves the outbound journal and becomes
an inbound delivery into this peer's own mod (§9.4). **Every remaining automatic bounce is a
case where no custody moved** — a payload refusal, an entry that reached nobody and has no lane,
or an exact `NOT_FORWARDED` chain that reached its bound. Thus, an automatic bounce is never a
guess (§9.4; amended — §31, B46).

**A late `MIGRATION_ACK` against a `lost` tombstone is GOOD NEWS, and it must still be read.**
It says the organism did arrive and its acknowledgement outran the deadline. **Exactly one
organism exists** — nothing was bounced home in the meantime — so the sidecar logs it, counts
it, and treats it as the signal that `forwardTimeoutMs` is set shorter than this map's slowest
honest answer. An implementation that swallows an ACK for an unknown `migrationId` silently
throws that signal away. ~~It is the accepted duplication case of §9.3 announcing itself.~~
**Superseded — §25, B37: there is no duplication case left to announce.**

**What counts as proof of no custody**, and nothing else does:

| Evidence | Why it proves it |
|---|---|
| The entry is `pending` | The frame was never handed to anybody. The sender's own record is sufficient. |
| A **sidecar-generated** `MIGRATION_NACK` with a peer-local code — `OVERLOADED`, `SIM_SIZE_MISMATCH`, `MOD_ABSENT` | §6.8 forbids a sidecar to NACK after it has durably journaled, so a NACK *is* the receiver stating it took no custody. |
| A relay `SLOT_VACANT` or `PEER_OFFLINE` with `neverForwarded: true`, a matching `relaySessionId`, and no same-session receipt | The migration-wide proof is exact while it remains true. Any receipt for that relay session contradicts it. |
| A queue-full relay `NOT_FORWARDED` with a matching `refusedAttempt` and `relaySessionId`, plus no same-attempt receipt (added — §31, B46) | The relay rejected this exact `(destSlot, rerouteCount)` before queue acceptance. `neverForwarded` must be present for mixed-version discrimination, but its migration-wide value can be false after an earlier attempt. |

| **Not** evidence | Why not |
|---|---|
| Silence, of any duration | The exact shape of the dangerous case. |
| `neverForwarded: true` with a **different** `relaySessionId` | The relay restarted; it cannot speak for what the previous process forwarded. |
| `SLOT_VACANT` or `PEER_OFFLINE` with `neverForwarded: false`, or `NOT_FORWARDED` with false but no exact attempt correlation | The legacy migration-wide statement is not proof. A queue-full NACK can still prove its exact attempt only when every B46 correlation field matches. |
| A relay NACK carrying **no** `neverForwarded` field | It is proof-free. This is required for graceful drain and can come from an older relay. Treat it as no proof. |
| Queue-full `NOT_FORWARDED` with no `refusedAttempt` | A 4.1 relay cannot identify the attempt. This is safe mixed-version degradation and authorizes no action. |
| A stale or mismatched `refusedAttempt` | A delayed NACK can describe an older destination or count. It cannot move the current attempt. |
| The `code` alone, on any single attempt | A code does not correlate a delayed answer to the current journal state. |
| `SLOT_VACANT` on its own | It proves the destination will never return, which is a reason to stop retrying — not a statement that nothing was ever delivered there. |
| **The absence of a `FORWARD_RECEIPT`** (added — §22, B26) | A receipt that was never sent, was dropped from a full outbound queue, or was lost with the session is indistinguishable from a forward that never happened. **A missing receipt is silence, and silence is never proof in this contract.** The receipt is evidence in exactly one direction: holding one means the frame *was* forwarded. Not holding one means nothing at all. |
| A proof contradicted by a `FORWARD_RECEIPT` for the same session and destination (amended — §31, B46) | The statements cannot both be true for this attempt. Safety keeps the entry `sent`. A receipt for another attempt is not a contradiction. |

**The rule, as the design states it** (`m4_considerations.md`, Question 2):

| Journal entry state | Evidence | Action |
|---|---|---|
| Never forwarded | `pending`, or a relay `SLOT_VACANT` / `PEER_OFFLINE` carrying matched proof | **Re-route** along the same axis to the current effective neighbour. Keep the `migrationId`. Rewrite `destSlot`. |
| Destination transport refused before acceptance | Queue-full relay `NOT_FORWARDED` with a matching `refusedAttempt` and `relaySessionId`, plus no receipt for that session and destination (amended — §31, B46) | In one durable update, record `refused`, the destination in the tried set, and the first refusal deadline if it does not exist. Clear the old attempt's session and `sentAt`. **Continue the same-axis walk after that destination.** Try the next compatible, untried slot. Bounce once when no such slot remains, `maxReroutes` is reached, or the first deadline expires. |
| Relay graceful drain | Proof-free `NOT_FORWARDED` | Keep the entry `sent`. Do not add a tried slot, start a refusal deadline, re-route, or bounce. Wait for reconnect or the normal loss outcome. |
| Refused for a peer-local reason | `MIGRATION_NACK` with `OVERLOADED`, `SIM_SIZE_MISMATCH` or `MOD_ABSENT` | **Re-route.** The receiver stated it took no custody, and another slot accepts the same organism. |
| Refused for a payload reason | `MIGRATION_NACK` with `INVALID_PAYLOAD`, `KIND_UNSUPPORTED`, `VERSION_UNSUPPORTED` or `MALFORMED_MESSAGE` | **Bounce home.** Every slot refuses this organism, so the map is not the answer. **`VERSION_UNSUPPORTED` is the first of B31's four kept gates** (added — §22, B31): the importing mod refuses a payload whose game version it has no dialect for, permanently, and D22's diagnostic-only rule does **not** retire it. `contract-a.md` **§21, A48** owns the mod-side statement — it keeps that document's §9.2 `VERSION_UNSUPPORTED` and its close `4002` as two of the four named exceptions, and states the diagnostic-only rule for `MIGRATE_OUT.gameVersion`, `parents[].gameVersion` and `MIGRATE_IN.gameVersion`; this row is the Contract B consequence and it is unchanged. |
| Forwarded, then silence | none | **Nothing, and then a record** (amended — §25, B37). The frame is **not** re-forwarded, the entry is **not** re-routed and the organism is **not** brought home. At `forwardTimeoutMs` the entry becomes a `lost` tombstone and `stats.lostForwardTotal` increments (§9.3). ~~Hold, then bounce: retry the recorded `destSlot` on the retry cadence — flat toward a dark destination, backing off exponentially toward a live one (§14, B5, B8).~~ |

**Re-route mechanics.**

| Rule | Statement |
|---|---|
| The key never changes | The `migrationId` is preserved. A re-routed frame that races a late delivery is deduplicated at whichever peer sees it second (§6.6 step 1) — this is what makes a re-route safe even when the proof was, against all the rules above, wrong. |
| The axis never changes | An entry that left through `E` stays on the east row walk, and one that left through `N` stays on the north column walk. For a never-sent or dark destination it uses the current `effective(edge)`. For a peer-local refusal or an exact queue refusal, it **continues the walk after that destination**. It skips the source and every tried or refused slot. The durable set prevents restart and compaction from creating a loop (amended — §31, B46). `exitEdge`, `exitPosition`, `velocity`, and `heading` are untouched. |
| Only `destSlot` is rewritten | Plus the `reroute` block (§6.6). The relay reads only its `count` for queue-refusal correlation. The receiver does not act on it. `timestamp`, the annex, and the body are the same bytes. |
| It needs a lane | With no effective neighbour on that axis there is nowhere to re-route to. An ordinary never-sent entry waits and bounces after `bounceTimeoutMs`. An exact queue-refusal chain bounces as soon as a complete current map proves that every compatible same-axis destination was tried. If the current map cannot prove exhaustion, it waits only for the first refusal deadline (amended — §31, B46). |
| It is bounded | `reroute.count` increments on each rewrite and MUST NOT exceed `maxReroutes` (4). A present count MUST be positive. The tried set prevents a destination from repeating. The monotonic count makes every `(destSlot, rerouteCount)` pair unique inside the refusal chain. Beyond the bound, the entry bounces home. The chain also has one deadline: the first proven refusal time plus `bounceTimeoutMs`. The deadline and tried set MUST NOT reset on reroute, retry, reconnect, restart, or compaction (amended — §31, B46). |
| A later send ends the safe bounce path for that attempt | Before the sidecar offers an original or re-routed payload to its socket writer, it durably records `sent` with fresh session and `sentAt` values. A crash or enqueue error after that record MUST NOT retry or bounce that attempt. The historical refusal deadline can remain, but it cannot move a `sent` entry. Only a matched refusal of this attempt, an answer, or the normal loss path applies (amended — §31, B46). |
| It is reported | Each re-route is a log line and a metric, and the frame carries its own `reroute.proof`. Part 2 of the exit test requires every in-flight entry to state which answer it took and why. |

**Why `pending` is worth a state of its own, and it matters MORE since §25's B37.** Without
it, an entry the sender never even managed to send would be indistinguishable from one that
was sent into silence, and the safe answer for the ambiguous case would be applied to the case
that is not ambiguous at all. That answer used to be *hold for a day*; it is now *lose the
organism*, so the distinction stopped costing a delay and started costing a life. **Most
re-routes in a real outage are `pending` entries**: the west neighbour noticed the gap before
it wrote anything, and every one of those still crosses.

**And it is why proof-based re-routing stays.** §25's B37 removes the answer to SILENCE and
nothing else. A `MIGRATION_NACK` that proves no custody moved is a **statement**, not silence;
re-routing under one cannot duplicate an organism, because the party that would have held the
second copy has said in as many words that it holds none. Removing the re-route as well would
lose an organism every time a destination was merely busy or briefly absent — the ordinary
case — to buy nothing at all. **The operator's switch, for a map that wants a single hop and no
second destination, is `maxReroutes` (§12): a NEGATIVE value turns re-routing off, and an entry
that would have re-routed bounces home instead.** It is a knob rather than a code path so that
the choice costs a restart and not a release.

### 9.3 The loss, and the deadline that only ever closes a record

**Amended — §25, B37. The bounded hold is gone**, and with it the clock, the accrual, the
retry that fed it, the automatic bounce it ended in and the one duplication case at-most-once
carried. This section is what replaced them, and it is shorter because it does less.

An entry in `sent` is an organism committed to one enqueue attempt with no proof of non-delivery.
**The sender does nothing about it.** It cannot tell a delivery whose acknowledgement was lost from a delivery
that never happened, it will not guess in either direction, and there is no action available
to it that is safe in both cases. It waits for an answer, and if none comes it writes the
organism off.

| Rule | Statement |
|---|---|
| The deadline | `forwardTimeoutMs`, default **300 000 ms (5 minutes)** (amended — §27, B42; 24 hours until 2026-08-19), measured from `sentAt` — the wall clock at the durable commitment to the **current or most recent attempt's** socket enqueue (§7.4; amended — §31, B46). A proven refusal clears this value before a pending alternate is listed or replayed. |
| **Nothing is re-sent** | A `sent` entry **MUST NOT** be forwarded again, to its recorded `destSlot` or to any other. One organism, one hand-over. The re-forward is what the bounded hold existed to bound, and removing it removes the bound's reason to exist. |
| **Nothing comes home** | At the deadline the sidecar **MUST NOT** deliver the organism to its own mod. That bounce was the accepted duplication case; there is no accepted duplication case. |
| At the deadline | The entry becomes a **`lost` tombstone**: `handoff: lost`, `status: done`, the reason recorded on the entry. The sidecar increments `stats.lostForwardTotal` (§6.3.1) and logs **one loud line at error level**, naming the `migrationId`, the `entityId`, the `destSlot`, the exit edge and whether a `FORWARD_RECEIPT` was ever held for it. |
| Why a tombstone and not a deletion | So a late `MIGRATION_ACK` is still recognised (below), and so `--list-inflight` stops showing an entry nobody can act on. It is purged with every other tombstone on `exportRetentionSeconds`. |
| It is a **wall clock**, and that is now safe | The accrual it replaced existed because expiry MOVED AN ORGANISM: a sender that had never observed the destination dark must not wake from a night's sleep and bounce something that was on its way. Expiry now only closes a record, so a sidecar that slept through the deadline records a loss that had already happened, and the two-condition clock buys nothing worth its complexity. |
| A restart | Resumes the same deadline, because `sentAt` is durable (§7.4). It neither restarts it nor resolves it. A pre-§25 journal has no `sentAt`; the entry's own `journaledAt` stands in, which is earlier than the forward it stands for and therefore the safe direction. |
| Reporting | **A loss is a fact the operator reads, not a silent repair.** The status page and `ringstat` both show `lostForwardTotal` per peer, and `--own-slot` shows this world's own unresolved depth beside it. |

**A late `MIGRATION_ACK` is the map telling you the deadline is too short.** The far peer took
custody, its acknowledgement took longer than `forwardTimeoutMs` to arrive, and the entry was
already written off. **Exactly one organism exists** — nothing was bounced home in the meantime
— so the record was wrong and the world was not. The sidecar **MUST** log it, name the
`migrationId` and the `entityId`, and count it; the remedy it names is to raise
`forwardTimeoutMs`, not to change anything about custody.

**What was given up, stated so nobody rediscovers it as a bug.**

| Case | Before | Now |
|---|---|---|
| The destination took custody, died before its ACK, and never returned | Held 24 h, then bounced home. One organism, and it survived. | **Lost.** |
| The destination took custody, died before its ACK, and returned after the timeout | Held 24 h, bounced home, then the far peer replayed its journal: **two organisms**. The accepted exception. | One organism, at the destination. The record says lost and a late ACK corrects it. |
| A `sent` entry left by a sender that crashed after its durable send commit and before the relay's answer | The retry re-asked, the relay answered `neverForwarded: true`, and the entry re-routed within a retry interval (§14, B5). | **Lost unless the exact current-attempt proof was already journaled.** The sender does not retry after restart. A crash after enqueue but before the NACK remains `sent` (amended — §31, B46). |
| A `pending` entry, or one refused by a statement | Re-routed. | **Re-routed, unchanged.** No custody moved, so nothing can duplicate. |

**The third row is the real cost of this set and it is named rather than buried.** B5's whole
argument was that the retry is how a non-delivery proof reaches a sender that already forwarded,
so removing the retry makes `sent → refused → re-route` unreachable for an entry a *previous
process* wrote. The window is narrow — the relay answers a declined forward in milliseconds, so
only a crash between the durable send commit and the answer lands in it — and the owner took the trade
knowingly on 2026-08-17, against the simplicity of a system with no duplication case in it at
all. **A conforming implementation MUST NOT re-add the retry to recover it.**

**The manual escape hatch, and it is now the only one.** `multiverse-sidecar
--release-inflight <migrationId> bounce|drop` resolves one unresolved entry by hand before the
deadline. It is the custody twin of `--release-slot`, it runs on the machine that holds the
journal, and it **MUST** print the duplication risk in its own output before acting.
**`bounce` on an entry in `sent` is the ONLY WAY LEFT TO DUPLICATE AN ORGANISM ON THIS MAP**,
and the printed warning MUST say so: nothing automatic does it any more, so the operator is
not firing one of two mechanisms but the only one. `--list-inflight` (§7.5) is how the operator
finds the entry, and the `FORWARD_RECEIPT` block it prints (§6.12, §22 B26) is the evidence
that decision turns on.

**A destination whose slot is vacant is lost at the deadline like any other, and that is
deliberate.** A released slot never returns (§2), so there was never anything to wait for; the
wait existed for the possibility that custody had already moved to a peer that is gone, and
that possibility now resolves as a loss rather than as a bounce. One rule, one deadline, one
place to reason about it.

### 9.4 Bounce-back

**Custody chain step 6 said a remote NACK or a timeout re-injects the organism into the origin
sim, and the timeout half was where duplication lived. THE TIMEOUT HALF IS GONE** (amended —
§25, B37). Every remaining bounce is a case where **no custody moved**, so a bounce is never a
guess and can never make a second organism (`contract-a.md` §13, A6, amended in the same wave):

| Situation | Origin sidecar's action |
|---|---|
| A sidecar `MIGRATION_NACK`, or a relay NACK with exact proof | **Bounce**, or re-route first where §9.2's table says so. A sidecar NACK is sent only before durable custody. A relay NACK proves no custody only under §9.2's exact current-attempt rules. |
| An exact queue-refusal chain exhausts compatible untried same-axis destinations, reaches `maxReroutes`, or reaches its first refusal deadline (amended — §31, B46) | **Bounce once.** The tried set and first deadline are durable. A repeated or stale NACK, reconnect, restart, or compaction cannot create another bounce or restart the bound. |
| The forward never reached a live peer — the relay link is down, or `destSlot` is vacant — for longer than `bounceTimeoutMs`, **and route-around has no lane to offer** | **Bounce.** The frame was never handed to anyone. |
| The forward reached a live peer and no answer came back — destination **live** or **dark**, it makes no difference | **Nothing, then a `lost` record at `forwardTimeoutMs`** (§9.3). Not a re-forward, not a re-route, not a bounce. ~~Hold and re-forward toward a live destination, backing off to `forwardRetryMaxMs` (§14, B8); hold with §9.3's clock toward a dark one and bounce at the timeout.~~ **Superseded — §25, B37.** |
| An operator runs `--release-inflight <id> bounce` on an entry in `sent` | **Bounce**, and it is the only bounce in this table that may duplicate an organism. The command prints the risk before it acts (§9.3). |

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
3. **Re-evaluate the outbound entries under §9.2.** A `pending` entry re-routes. An exact
   refusal chain keeps its tried set and first deadline (amended — §31, B46). A `sent` entry
   keeps its `sentAt` and waits for an answer it will probably not get (amended — §25, B37:
   it used to retry its recorded destination and keep its accrued hold).
4. **Reassert custody against the mod.** A new `sessionId` triggers `contract-a.md` §7.4.

**No backlog waits at a returning peer, and less than ever** (amended — §25, B37). Its
neighbours re-routed only the entries they had never forwarded, and the entries they *had*
forwarded are not re-sent at all, so nothing accumulated behind the gap in *their* journals. What did
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

**A store write is rename-into-place, and a failed one MUST clean up after itself**
(added — §20, B20). The write lands in `<hash>.json.tmp` and is renamed over
`<hash>.json`. On **any** error before the rename the writer MUST remove that scratch
file, and the expiry sweep MUST also collect scratch files old enough that no live write
can still own them — a process killed between the write and the rename cannot run its own
cleanup, and the sweep otherwise never looks at anything but `.json`.

The rule reads as housekeeping and is not. The store is content-addressed, so a failed
write retries under the **same** name and leaves the same corpse again: on 2026-08-08 a
full disk turned one `ENOSPC` into **15,119 zero-byte scratch files** across the
deployment's six stores, every one an inode spent on nothing at the moment inodes and
blocks were what the rig had run out of. The archive's own store is bounded by neither
tunable above — it is the record, and nothing evicts from it (§20, B20). **Since Decision 3 it
may be given a retention horizon instead of a cap** (amended — §23, B33): still no
least-recently-served **cap** and no byte budget, but an operator-set age past which a **blob**
is pruned — **unset by default**, and never, at any setting, applied to the ledger.
**The ledger has its own window since 2026-08-17** (amended — §26, B40), read from the same
number: the genome horizon, the fetch queue's retirement and the ledger's window are one value
(§26, B40). It is still not the genome horizon that touches a line — it is the ledger's own
knob, and the aggregate the lines fold into is kept forever (§26, B39).

**Fetch behaviour, archive side.**

| Rule | Statement |
|---|---|
| Never block | A migration record is written when the envelope arrives. A missing genome is a **gap** on that record, never a reason to delay or refuse a record. |
| Ask the source first | The envelope's `sourcePeer` hashed the blob and cached it, so it is the peer most likely to have it. |
| Then ask around | On `unknown_hash`, try other live peers in structural order, one at a time. |
| Rate limit | At most `genomeRequestsPerMinute` (30) requests from one requester to one peer. The answering sidecar enforces the same limit and answers `rate_limited` above it. **It is a published knob under `contract-b/4.0`, not a compiled constant** (amended — §22, B24): §3.3 names it `maxGenomeRequestsPerMinute`, the relay publishes the value it is running with (§6.2, §6.5), and an operator can retune it from the metric that measures it (D20). Nothing about the limit's behaviour changes; what changes is that it can be moved without a rebuild, which on a public archive is the difference between a support answer and a release. |
| Pace it, and bound the pass (added — §21, B21) | The rate above is **not** a budget. One pass of the queue examines at most `genomeScanPerTick` gaps, round-robin over a stable order and resumed where the last pass stopped; releases the requester's own lock every `genomeScanChunkSize` gaps, so its read loop is never starved; and sends at most `genomeRequestsPerTick` requests, carrying at most `genomeInFlightPerPeer` unanswered to any one peer. **None of it changes when a gap is retried** — see §21 for the incident that made a 64,736-entry backlog cost 7,789 ledger records. |
| Retry schedule | 1 minute, 5 minutes, 30 minutes, 6 hours, then daily. Reset the ladder when the map's membership changes — a peer that just came back may hold what nobody had. |
| Keep the hash forever | A hash with no genome is a permanent, useful record: it is still a lineage-graph node, and a fetch that failed for a year can succeed tomorrow. **The hash, and no longer always the asking** (amended — §23, B34): where a retention horizon is set, a gap whose **crossing** is older than it is retired from the retry set and counted as `genomeGapsExpired`, because the only blob such a fetch could win is one the next eviction pass would delete. The **record** is untouched — the hash stays a lineage node, stays in the gap report and stays `[MISSING]` in the read path — and with no horizon set the ladder runs forever exactly as this row has always said. |
| Verify | Recompute the hash of every fetched blob (§6.10). |
| Report | The gap report lists every hash no peer has served, with its first-seen time and attempt count. It is the archive's honest statement of what it does not have. |

**Species names on the record** (added — §15, B10). The archive records the `species` block of
§6.6 against the migration it arrived on, and that is the only place on this wire a species
name is used for anything.

| Rule | Statement |
|---|---|
| Recorded verbatim | Both names, and the parent pair when present, stored byte for byte as they arrived. The archive **MUST NOT** trim, case-fold, normalize or re-case a name; the destination mod's match is a byte comparison (`contract-a.md` §5.7), and a ledger that tidied its copy would stop describing what actually happened. |
| A ledger fact, not a resolution | The block says what the **origin world** called this migrant at the moment it left. It is not a claim about the destination, and the archive **MUST NOT** resolve a name against any world's registry, merge two records because their names match, or rewrite an earlier record when a later envelope disagrees. Species resolution happens in exactly one place in this system, and it is the importing mod. |
| A name is not a permanent lineage identity (amended — §29, B44) | The raw rule above stands. For the **derived genealogy only**, the archive groups ordered evidence into immutable lineage instances. One instance is one normalized name on one recorded parent path. Reusing a name on an incompatible path creates a second instance. It **MUST NOT** replace the earlier edge. This grouping does not merge, rewrite, or resolve either raw record. |
| Absent is absent | A migration with no block records **no species** — never `"unknown"` as a value, never an empty string standing in for one. That is §10.1's unknown rule applied to a new field. |
| Dedup unchanged | A `MIGRATION_PAYLOAD` still deduplicates on `migrationId` alone (§5.1). The block is part of the record that key names, never part of the key. |
| The win | Before this the ledger could name only hashes, so "which species crossed, and when" was a question the archive held the data to answer and not the labels. It now travels on every hop that carries a block. |
| Not the page's species view | **Amended — §16, B12.** The page *does* render species now, and **not from here**: its species view is the live census in `stats.species` (§6.3.1), which describes who lives in a world. This ledger describes who **crossed**, and it remains **not an input to any abundance claim** — a database built from migrations holds migrants and their ancestors, never a resident population (D11, below). The two also spell a name differently on purpose: the ledger's copy is normalized at the source (`contract-a.md` §16, A34) and the census's is raw (`contract-a.md` §17, A36), so a consumer that joins them normalizes **for the comparison only** and rewrites neither. |

**The game version on the record is diagnostic, and this is the record it is diagnostic for**
(added — §22, B31). The archive already writes `body.version` against every migration it is
copied — the living deployment's ledger lines carry `"gameVersion":"0.6.3.1"` today — and D22's
refinement of 2026-08-11 is what that field is *for*: **carry it, decide nothing from it.** The
archive **MUST NOT** refuse a record, split a lineage, filter a fetch, order a queue or mark a
peer on the strength of it. It is the field that makes a first cross-version incident legible
from the record instead of reconstructed from memory, and until one happens there is nothing
here to design (D22, *the trigger is an actual cross-version load failure*).

**The known limit, restated because it is load-bearing** (D11): a database built from
migrations holds migrants and their ancestors, never the resident population of a peer.
Census uploads would close the gap and are not M4.

**And one limit that is now on somebody else's timetable** (added — §22, B27). Nothing evicts
from the ledger or the genome store (D11, §20 B20) and a public archive's growth scales with
peer count, so the operator who hosts one has to size a disk **before** it fills rather than
after — the 2026-08-08 ENOSPC outage happened on a volume one person was watching. The
retention rule itself, and the per-peer growth arithmetic that goes to whoever hosts the
archive, are **WP3's deliverable and not this contract's**: `m5_considerations.md` decision 3
ratifies that the *deciding* happens in M5 and D24's announced ending is its deadline. What
this document states is the constraint that decision has to live inside — **nothing here may
evict**, and a retention rule that contradicts that is a change to D11 rather than a
configuration of it.

**The decision landed on 2026-08-12, and it is that change, on one half** (amended — §23, B33).
The owner's answer is *the ledger forever, the genome blobs to a horizon*, so **§23 is the
recorded amendment this paragraph asked for**: the ledger's never-evict rule stands exactly as
written above and D11 is untouched, while the **genome store** gains an operator-set retention
horizon that is **unset by default at this contract's level** and turned on by a deployment.
The growth arithmetic stays WP3's deliverable and now has a second number beside it — the age at
which the bytes stop accumulating. Risk 7 becomes a **policy**: a genome nobody fetched inside
the horizon is permanently unfetchable, and the archive counts what it abandoned (§23, B34).

**The second half landed on 2026-08-17, and it is the other change this paragraph asked for**
(amended — §26, B39/B40). The owner's ratification splits **the record** from **the lines that
carry it**. What §10 has always meant by *the record* — who crossed, how often, from where,
descended from what, first and last seen when — becomes a **durable aggregate that is written
down and kept forever**, and it is what every surface and every evidence item reads (§26, B39).
The **per-crossing lines** in `migrations.jsonl` gain a window equal to the genome horizon, are
compressed once closed, and leave the host only after an off-host copy is confirmed (§26, B40).
**Nothing here may evict from the record** is therefore still the rule of this section, and the
sentence above about the ledger now reads as a rule about the aggregate rather than about every
line behind it. §22's B30 is untouched and is worth reading beside this one: **a window ages
lines by date and is not a takedown mechanism** — it cannot name a peer, a species, a hash or
an organism, and M5 still promises removal from the view and not from the record.

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
| The genealogy keeps name reuse visible (amended — §29, B44) | The census remains the only abundance source. The genealogy can bind one census row to a recorded lineage instance with the ordered world evidence in §29. If worlds bind the same live name to different instances, the page **MUST** show separate rows with the same display name. It **MUST** label the split. If the record cannot select one instance, the page **MUST** label the identity unresolved. It **MUST NOT** call that state "no recorded ancestry". |
| A world's speed and its pacing are settings, and unknown beats the default (added — §18, B17) | The page may show each world's `timeScale` and the `inboundRatePerSimMinute` / `pacedDepth` pair (§6.3.1), because a depth is only readable against the cap it is queued behind and a simulated-minute cap is only readable against the speed that spends it. Every rule above binds them, and **the unknown rule binds them hardest**: a peer that publishes no cap renders as **unknown**, never as the shipped default. `timeScale: 0` is the opposite case and the page **MUST** keep the two apart — a world standing still is a reading, and a world that has not said is a gap. |
| A world's settings are what it was told to do, they are read-only, and unknown beats the default (added — §19, B19) | The page may show each world's `modVersion`, `contractAVersion`, `migrationExclude`, `saveMinutes`, `saveKeep`, `saveOnQuit` and `worldWrapping` (§6.3.1). They are the **cause** behind numbers the page already shows, and each has a reading the page **MUST NOT** flatten into a gap: `saveMinutes: 0` is a save timer that is **off** and is the explanation for an absent `lastSave`; a present `migrationExclude: []` is a policy that is **off**, and a populated one is why a world can be full of a species that never appears on a lane (§17, B14 names that shape); `worldWrapping: false` is a world not containing its own organisms. **Absent is unknown in every case, and the page MUST NOT substitute a shipped default** — the one it would reach for, `saveMinutes: 10`, would claim a world is being saved when its timer may be off, which is the most expensive wrong number this page could print. They are also **read-only**: the page renders them and offers no way to change one (`contract-a.md` §19, A43). |
| **Every string on this page is attacker-chosen, and escaping it is a testable obligation** (added — §22, B30) | The page and `ringstat` **MUST** escape every peer-supplied string for the surface they render it into — HTML, an HTML attribute, a URL, JSON embedded in a script, and a terminal's escape sequences — and **MUST NOT** render one as markup under any circumstance. The strings are named so nobody has to infer the list: `species[].genericName` and `.specificName` (up to 64 UTF-8 bytes each, `speciesCensusMax` of them per peer), `migrationExclude[]`, `modVersion`, `contractAVersion`, `lastRefusal`, `lastSave.name`, and the two the contract never counted because they predate the concern — **`peerId`**, and the world name a player chose. **A rule written in a contract is not code**, so this one ships with a **test in CI and not an inspection by eye**: a peer reports a species named with markup, and the rendered page and `ringstat`'s terminal output are both asserted against it. That test is WP7's, and it is the only form in which this row is true of a running system. |
| **A version string is never a capability decision, and a page is where that gets forgotten** (added — §22, B30) | §13 item 7 states it of `contractAVersion` and the rule is repeated **here**, where the person implementing a page will actually read it: a reader **MUST NOT** parse `contractAVersion`, `modVersion` or a peer's `gameVersion` into a capability or a refusal. A peer that can choose the string can choose the capability it claims. **Detect a feature by the presence of its field** (`contract-a.md` §3.1) — `stats.species` absent means unknown, not "old mod", and the page renders unknown either way. The same prohibition binds the envelope's game version (B31), and the four kept exceptions to it are gates in the relay, the walk and the mods — **never in a page**. |
| **Suppression happens in the view, and the record keeps everything** (added — §22, B30) | Beyond injection there is moderation, and the honest boundary has to be stated before somebody is asked for a takedown. The page and `ringstat` MAY apply an **operator-side deny list at render**, suppressing a string without evicting the world that produced it: it needs no peer cooperation, no wire field and no contract change, and eviction stays available through B28's admin path for a peer that will not stop. **What cannot be promised is removal from the record**: D11 and §10 make the ledger a thing nothing evicts from, so **M5 promises removal from the view and explicitly does not promise removal from the record**. Anything stronger is a change to D11's never-evict rule and belongs in a decision row, not in a support reply (`m5_considerations.md`, DQ7). |
| **The relay's limits render beside the behaviour they bound** (added — §22, B24) | The page may show `limits` and `minContractVersion` from `PEER_STATUS` (§6.5). They are the relay's own configuration, so unlike every stats field they are **authoritative rather than reported** — but the unknown rule still binds their *effects*: a peer's frame or claim rate the page did not observe is unknown, never zero. The value is the same one §18's B17 found for pacing: **a number is only readable against the cap it is measured on**, and a `lastRefusal` of `contract_version_below_minimum` is only actionable beside the minimum that refused it. |
| The recent-hops feed is delivery history, and it animates rather than counts (added — §17, B14; corrected 2026-08-23) | The page may show **which species a receiving game acknowledged on which lane, just now**, as a bounded feed of the last ~60 seconds correlated from the `MIGRATION_PAYLOAD` and `MIGRATION_ACK` copies the archive already receives. A payload is an **offer**, not proof of delivery: an admission NACK produces no glyph, and a rerouted migration appears only at the destination whose ACK matches its current attempted slot. The page **MUST NOT** infer delivery by differencing the payload-derived lane counters when this feed is unavailable. It is B12's third row exercised — *history, labelled as history* — and every rule above binds it: a delivery whose payload carried **no** species block renders as the **neutral glyph**, never a guessed name and never omitted; the feed is **never summed**, into a census or into anything else; and it must be bounded in **both** time and count, because the status view is serialized verbatim into the durable metrics file once a minute. |

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
**since §16, which species live in each world and in what numbers**, **since §18, how
fast each world runs and the cap its arrivals are paced behind**, and **since §19, what each
world was configured to do — its mod and protocol versions, its exclusion list, its save policy
and its wrap**. Every one of those is a field of §6.5 or §6.3.1, or is derived from them by
§8 — which is why those two sections carry fields that no routing decision reads.

**Since §17 it also asks the page to show the lanes moving** (added — §17, B14). When a
migration lands, the map animates that species' glyph along the lane's arrow. The inputs are
not new — the archive has recorded a species block on every migration since §15's B10, and it
has counted per-lane hops since M4 — and no wire field is added. What is new is the join, and
it is the cheapest verification the operator surface has ever had: **two-way lanes look like
glyphs travelling both ways along one arrow, and an excluded species (`contract-a.md` §18,
A39) looks like a species that is everywhere in the census and never on a lane.** **Since §19
the page can also say *why*** (added — §19, B19): that species is named in the same world's
`migrationExclude`, so the shape and its cause sit on the same card instead of being an
inference a reader has to make.

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
   parses a body, never validates a payload, never indexes a genome, and never takes organism
   custody. The walk reads the registry it already keeps. The record is a set of ids it already
   routed. **Since B43, its transport may briefly retain byte-identical payload frames**
   (amended — §28, B43). Frame and retained-byte bounds make that transport state finite, and
   no queued frame becomes a domain object or a routing index (§3.3).
7. **`entryEdge` is still not on the wire.** The receiving sidecar derives it from `exitEdge`
   by the opposite-edge function (§6.6), and for a bounce-back from its own peer's exit edge.
   Both are facts the receiver already knows about itself, and sending it would let a sender
   dictate a receiver's geometry.
8. **The non-delivery proof is a field on an existing message, not a new message pair**
   (§5.2, §6.8). A `FORWARD_QUERY` / `FORWARD_STATUS` pair was the obvious alternative and it
   is worse: the sender's forward **is** the query, so a separate ask would be a second
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
12. **The relay's pre-act report cannot list unresolved entries, and the split is stated
    instead** (§7.5). Question 3 asks `--release-slot` and `--handover-slot` to list the entries
    that name the slot. Journals live on the peers' own machines and D2 keeps custody local, so
    a relay that could enumerate them would be a relay that reads journals — which is a bigger
    change to D1 than anything else in this milestone. The relay prints the map-side
    consequences; `stats.lostForwardTotal` (§6.3.1, amended — §25 B37) shows which peers have
    already lost anything addressed to it on
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
| `authFailuresBeforeCeiling` | `5` | client | Consecutive HTTP 401s before the backoff is pinned at the ceiling (§3.1). **Unchanged in value and changed in meaning** (amended — §22, B22): a 401 now means *this peer's credential* was refused, so the log line names the remedy and the peer waits for a person rather than for the relay. |
| `relayTLSMinVersion` | `1.2` | relay | **New in `contract-b/4.0`** (added — §22, B23). The lowest TLS version the relay's own listener accepts. A relay behind a fronting proxy does not own this; the proxy does, and B23 says the wire-visible behaviour is what this contract specifies either way. |
| `statusCoalesceMs` | `250` | relay | Minimum spacing between `PEER_STATUS` broadcasts, and between grants to one peer. The last frame of a burst is always sent (§7.2). **It is now the floor of a window that widens** (amended — §22, B29). |
| `statusCoalesceMaxMs` | `2000` | relay | **New in `contract-b/4.0`** (added — §22, B29). Ceiling of the coalescing window under sustained churn. The window doubles from `statusCoalesceMs` toward this value while a window sees more than `statusChurnBurstThreshold` registry changes, and narrows one step after a quieter one. It bounds the broadcast **rate**, which is what a public map's `slotCount` stats blocks per frame make expensive (§7.2). |
| `statusChurnBurstThreshold` | `8` | relay | **New in `contract-b/4.0`** (added — §22, B29). Registry changes inside one window that make the relay widen it. Sized above what a single peer's join or departure produces, so an ordinary event never widens the window and a churn storm always does. |
| `minContractVersion` | *unset* | relay | **New in `contract-b/4.0`** (added — §22, B25). The lowest `protocolVersion` this relay admits, published in `HANDSHAKE_ACK` and `PEER_STATUS`. **Unset means no minimum, and that is the default**: a floor is a deployment decision, and a relay that has not made one must not enforce a guess. It is raised only **after** the release that satisfies it is published (D25). A **compatibility** control, never a security one (§6.1). |
| `statsBroadcastIntervalMs` | `5000` | relay | **New in M4** (added — §14, B4). The §6.5 timer that republishes `PEER_STATUS`, and re-sends any changed `SECTOR_GRANT`, because **stats change without the registry changing**. §6.5 named it and this table did not define it. Set to `statsIntervalMs`, the cadence at which the stats it carries arrive: a faster timer would republish the same block, a slower one would age it. It is a compiled default with no flag and no environment variable. **It is the ONLY thing a stats arrival may cause to be published** (amended — §24, B36): a relay that also scheduled a broadcast on arrival broadcasts at the peer count's rate rather than at this one's. |
| `wireCompression` | `on` | relay | **New after `contract-b/4.0`** (added — §24, B35). Whether this relay **offers** `permessage-deflate` on a Contract B upgrade (§3). `--ws-compression`, `MULTIVERSE_RELAY_WS_COMPRESSION`. It is a knob for D20's reason — the transfer bill is a number only the operator can see — and it is a **complete** off switch on its own, because the extension needs both ends: a relay that stops offering it returns the whole map to uncompressed frames as each peer reconnects, with no participant action. **A client has no matching knob and MUST NOT be given one**, for the reason B23 gives its TLS verification none: nothing in a participant's hands measures this. |
| `statsIntervalMs` | `5000` | sidecar | Minimum spacing between stats-bearing `PING`s (§6.11). |
| `statsStaleMs` | `30000` | archive | Age at which a `stats` block renders as unknown rather than as state (§10.1). |
| `forwardRetryMs` | `5000` | sidecar | Cadence at which an entry with no durable send commitment is offered again (amended — §31, B46). It never retries a `sent` attempt, including after a socket enqueue error. A stale NACK MUST NOT postpone a pending current attempt. |
| ~~`forwardRetryMaxMs`~~ | ~~`60000`~~ | — | **REMOVED — §25, B37.** It capped a doubling backoff on the re-forward toward a live destination (§14, B8), and there is no re-forward to back off. |
| `bounceTimeoutMs` | `20000` | sidecar | How long an outbound entry that never reached a live peer, **and has no lane to re-route to**, waits before it bounces home (§9.2, §9.4). It is also the one absolute deadline for an exact queue-refusal chain (amended — §31, B46). That deadline starts at the first proven refusal and never resets. Graceful drain does not start it. |
| `migrationAckTimeoutMs` | `30000` | sidecar | Informational deadline for `MIGRATION_ACK`. **Purely informational since §25, B37**: expiry re-forwards nothing, and it never bounces (§9.4). |
| `forwardTimeoutMs` | `300000` | sidecar | **New in `contract-b/4.1`** (added — §25, B37; default lowered from `86400000` — §27, B42). How long a forwarded entry waits for its answer before the sender records it **lost** — 5 minutes since 2026-08-19; it carried `holdTimeoutMs`'s 24 hours until the owner cut it (§27). It is a **wall clock measured from `sentAt`** and a bookkeeping deadline: nothing is re-sent at it and nothing comes home, so a sender that slept through it records a loss that had already happened (§9.3). |
| ~~`holdTimeoutMs`~~ | ~~`86400000`~~ | — | **REMOVED — §25, B37**, with the bounded hold it timed. Its 24-hour value lives on in `forwardTimeoutMs`, which does something entirely different with it. |
| ~~`holdAccrualFlushMs`~~ | ~~`60000`~~ | — | **REMOVED — §25, B37.** There is no accrual to flush. |
| `maxReroutes` | `4` | sidecar | **New in M4.** Re-routes one entry may take before it bounces home (§9.2). The bound remains hard during an exact queue-refusal walk (amended — §31, B46). A NEGATIVE value turns re-routing off entirely (added — §25, B37). |
| `forwardRecordRetentionSeconds` | `172800` | relay | **New in M4.** How long the relay remembers a forwarded `migrationId` for the legacy migration-wide `neverForwarded` value. The attempt-scoped 4.2 queue proof comes from the rejected payload and does not weaken this record (amended — §31, B46). |
| `archiveDedupWindowMs` | `172800000` | archive | **New in `contract-b/4.1`** (added — §25, B38). How long the archive remembers a record's §5.1 duplicate key — 48 hours. A duplicate that arrives inside the window is refused; one that arrives later is appended a second time. The retention is a **floor**: at least this, at most twice it, and the memory is at most two windows of keys. `--dedup-window`, `MULTIVERSE_ARCHIVE_DEDUP_WINDOW`. |
| `inboundQueueMax` | `64` | sidecar | Shared with `contract-a.md` §10. Also the ceiling a paced backlog hits, which is what turns pacing into upstream backpressure (`contract-a.md` §7.5). |
| `exportRetentionSeconds` | `3600` | sidecar | Tombstone lifetime. Shared with `contract-a.md` §10. |
| `speciesCensusMax` | `32` | both | **New after M4** (added — §16, B11). Shared with `contract-a.md` §10. Upper bound on `stats.species` entries. A block that arrives with more is trimmed to the first 32 with one warning and `truncated: true`, never a refusal and never a close — the same answer `contract-a.md` §5.2 gives on the other wire. It is what bounds a `PEER_STATUS` broadcast carrying one census per slot (§6.3.1). |
| `maxFrameBytes` | `8388608` | both | Shared with `contract-a.md` §10. |
| `maxPayloadBytes` | `4194304` | both | Shared with `contract-a.md` §10. Applies to `body.bb8` and to a `GENOME_RESPONSE` body. |
| `archiveQueueMax` | `1024` | relay | Copied frames buffered per subscriber before the oldest is dropped (§5.1). |
| `maxConnectionsPerPeer` | `2` | relay | **New in `contract-b/4.0`** (added — §22, B24). §3.3. Simultaneous authenticated connections per `peerId`; the second is the `4006` overlap during a reconnect. |
| `maxConnectionsPerAddress` | `8` | relay | **New in `contract-b/4.0`** (added — §22, B24). §3.3. Deliberately loose — the rig itself runs five peers on one machine. |
| `maxFramesPerSecond` | `50` | relay | **New in `contract-b/4.0`** (added — §22, B24). §3.3, per peer, all types. Sized for a migration burst, not for a steady rate. Values below `8` are unsupported since B43 (amended — §28, B43). |
| `maxBytesPerSecond` | `4194304` | relay | **New in `contract-b/4.0`** (added — §22, B24). §3.3, per peer. It stops `maxFramesPerSecond` being evaded with maximum frames. Its numeric value also bounds retained migration bytes per destination connection (amended — §28, B43). |
| *destination migration queue* | `F` frames and `B` retained bytes | relay, derived | **Not a knob and not published separately** (added — §28, B43). `F = maxFramesPerSecond` and `B = maxBytesPerSecond`. Both bounds apply independently to each live destination connection (§3.3). |
| *destination migration write rate* | `max(1, floor(F / 8))` frames/s | relay, derived | **Not a knob and not published separately** (added — §28, B43). Evenly spaced and owned by destination identity across reconnect overlap. At most `7` ready ordinary or control frames pass a due migration. |
| *relay-wide paced-drain deadline* | `18000` ms | relay, fixed | **Not a knob** (added — §28, B43). One shared deadline covers parallel graceful drain of all connections. The relay force-closes and drops any remainder at expiry while retaining conservative attempted-forward records (§5.2). |
| `maxClaimsPerMinute` | `12` | relay | **New in `contract-b/4.0`** (added — §22, B24). §3.3. Above it a claim is answered `reason: "rate_limited"` and the connection is **not** closed (§6.4). |
| `maxSubscribers` | `4` | relay | **New in `contract-b/4.0`** (added — §22, B24). §3.3. Bounds the fan-out cost; B27's grant is what bounds the trust. |
| `credentialVerifierStore` | `<data-dir>/peers.json` | relay | **New in `contract-b/4.0`** (added — §22, B22). Where the per-peer **verifiers** and their grants live. It holds no recoverable secret (§3.1) and it is the third file the operator must back up, beside `ring.json` and the archive's durable set (D24, DQ2). |
| `genomeRequestTimeoutMs` | `15000` | requester | How long a requester waits for a `GENOME_RESPONSE` before it counts the attempt as failed. **Requester-side only, and deliberately the only entry** — see the note below. |
| `genomeRequestsPerMinute` | `30` | both | Per requester, per answering peer. Enforced on both sides (§10). **It is a RATE and it is not a burst bound, nor a bound on the work of one pass** — see `genomeScanPerTick` below and §21, B21. **It is `maxGenomeRequestsPerMinute` in §3.3's published table and it is a knob** (amended — §22, B24): the compiled constant `contractb.GenomeRequestsPerMinute = 30` is the worked example D20's rule was written about, and a public archive is the first deployment likely to need it moved. |
| `genomeScanPerTick` | `2048` | requester | **New after M4** (added — §21, B21). How many pending gaps one pass of the fetch queue may examine, walked round-robin over a stable order and resumed where the last pass stopped. It bounds the WORK of a pass, so the cost of a pass stops growing with the backlog. |
| `genomeScanChunkSize` | `256` | requester | **New after M4** (added — §21, B21). How many gaps may be examined under one acquisition of the requester's own lock. It is the yield: it bounds how long the requester's read loop can be kept from the socket, whatever the backlog. |
| `genomeRequestsPerTick` | `8` | requester | **New after M4** (added — §21, B21). How many `GENOME_REQUEST`s one pass may put on the wire. It bounds the BURST. Set far above what `genomeRequestsPerMinute` can sustain, so it never lowers the fetch rate. |
| `genomeInFlightPerPeer` | `8` | requester | **New after M4** (added — §21, B21). Unanswered `GENOME_REQUEST`s one requester may be carrying to one peer at any instant, independent of the rate. At `genomeRequestTimeoutMs` it still admits more than `genomeRequestsPerMinute` permits, so it too never binds before the rate does. |
| `genomeCacheRetentionDays` | `30` | sidecar | Genome cache lifetime, least-recently-served. |
| `genomeCacheMaxBytes` | `2147483648` | sidecar | 2 GiB cap on `<data-dir>/genomes/`. |
| `genomeRetentionHorizon` | *unset* | archive | **New after `contract-b/4.0`** (added — §23, B33). Decision 3's retention horizon: how long a genome **blob** is kept after it was last stored or last served, as a duration. **Unset means nothing evicts, and that is the default** — the M4 behaviour this document specified until §23, and the safe direction for a knob that deletes. A deployment turns it on: `--genome-horizon`, `MULTIVERSE_GENOME_HORIZON`, `720h` on the M5 hosted run. It applies to the **genome store only**; ~~`migrations.jsonl` is kept forever at every setting~~ — **amended — §26, B40**: no setting of *this* knob touches a line, and the ledger's own window is the separate knob in the row below. The **same** value retires a gap whose crossing is older than it (§23, B34) — one number, **three** mechanisms since §26's B40, or the archive re-fetches what it just evicted. |
| `ledgerWindow` | *unset* | archive | **New after `contract-b/4.1`** (added — §26, B40). How long a closed `migrations.jsonl` segment is kept on the host, measured from its newest record, as a duration. **Unset means the ledger is kept whole, and that is the default** — the behaviour of every release before §26, and the safe direction for a knob that deletes. A deployment turns it on: `720h` on the M5 hosted run, which is `genomeRetentionHorizon`'s own value because §26's B40 makes them one number. An implementation MUST refuse a negative value and MUST treat an unparsable one as unset. Setting it does **not** shorten the record: the aggregates behind the lines are kept forever (§26, B39), and a segment past the window is kept anyway until its off-host copy is confirmed. |
| `journalCompactMinutes` | `15` | sidecar | **New after M4** (added — §20, B20). How often the journal is rewritten to the entries it still holds. `--journal-compact-minutes`, `MULTIVERSE_JOURNAL_COMPACT_MINUTES`. |
| `logRotateMb` | `100` | all | **New after M4** (added — §20, B20). Size at which a process rotates its own log. `--log-file` names the file, `--log-rotate-mb` the cap, and a negative value disables rotation. With no `--log-file` the process logs to stderr and bounds nothing, which is the pre-M4 behaviour and still the default. |
| `logKeep` | `5` | all | **New after M4** (added — §20, B20). Rotated generations kept beside `--log-file`. The disk ceiling for one process is `logRotateMb × (logKeep + 1)`. |

**These three are a disk budget, and the system did not have one.** Every durable
file this contract names was append-only, and the two that had no bound at all
were the journal and the log. The journal compacted **only at `Open`**, so a
sidecar that stayed up accumulated every `create` record it had ever written —
payload included, up to `maxPayloadBytes` each — and the log was a shell redirect
with no size. On **2026-08-08** the living deployment's five local sidecars held
445 MB, 500 MB, 905 MB, 683 MB and 720 MB of journal for a live set of a few
hundred entries, beside 3.5 GB of log, and filled the root filesystem. Every
genome write in the rig then failed mid-`WriteFile`, leaving 15,119 empty
`<hash>.json.tmp` files behind (§10 now requires the failing writer to remove
its own scratch file, and the cache sweep to collect any a crash abandoned).

A compaction never reads the file it replaces — the in-memory state map *is* the
compacted content — so it costs one pass over the live entries and finishes in
milliseconds. It writes `journal.log.tmp`, fsyncs it, renames it over
`journal.log` and syncs the directory, which is the same discipline `contract-a.md`
§11.1 already required of `Open`; a crash before the rename leaves the original whole and a
crash after it leaves a journal that replays to the identical state. **Compaction
must preserve `sentAt` for a `sent` entry** (§9.3, amended — §31 B46).
`forwardTimeoutMs` is measured from it. A proven refusal or reroute MUST clear it before
compaction. A pending entry MUST NOT list or replay the prior attempt's loss deadline.

Compaction MUST also preserve the exact-refusal tried-slot set and first deadline (amended —
§31, B46). Dropping either field can retry a full destination or restart a bounded chain.

**What is still unbounded, and deliberately so:** the archive's
`migrations.jsonl` ledger and its genome store are the record of what happened
and nothing may evict from them (§10). They grow with traffic, not with uptime,
and an operator has to size a disk for them. `metrics.jsonl` grows with time at
one sample per `metricsInterval` per slot, which is small but also monotone.
**The ledger, permanently; the genome store, unless an operator says otherwise**
(amended — §23, B33). `migrations.jsonl` is unbounded by rule and always will be.
The genome store is unbounded by **default** — `genomeRetentionHorizon` is unset
— and an operator who sets one converts that term from a total into a steady
state of one horizon's worth of blobs. It is the only line of this paragraph a
configuration can move.

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
| The hold clock (§9.3, Risk 9) | **Removed — §25, B37**, and the risk it managed with it. A paced backlog at a live peer never accrued hold time; nothing accrues at all now, and a slow peer that eventually answers resolves its entry the moment it does. |
| `forwardRetryMs` (§9.2) | Unchanged in meaning, narrower in reach since §25's B37: it paces the offer of an entry that reached nobody, and a live destination that is draining faster has nothing to do with it. `forwardRetryMaxMs` is gone with the backoff it capped. |
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

**Two defaults are worth a second look before the rig runs long, and §25's B37 changed what
each of them trades.** `forwardTimeoutMs` is the owner's call and a policy, not a
measurement — it no longer trades a stranded organism against a duplication case, because
there is no duplication case. **It trades how long a record stays open**: too short costs
a wrong record and a late-ACK log line, too long costs a `--list-inflight` full of entries
nobody can act on — and, as 2026-08-19 proved, an export intake wedged shut, because open
`sent` entries count against contract A's `inboundQueueMax` and a day-long deadline holds that
gate closed for a day after any multi-world outage. **The owner cut it to 5 minutes for
exactly that** (§27, B42). `forwardRecordRetentionSeconds` and `archiveDedupWindowMs` are both
48 hours and are now sized against the same thing — how far behind this map's oldest peer may
be, which during the transition means a sidecar that still retries.

---

## 13. Open items for M5

**Every item below was placed by `m5_considerations.md` (2026-08-09), and six of the eight are
now closed by §22** (amended — §22). That document carried each one into a scope line, a work
package, a contract change or an owner decision; its *Scope* section cites this list item by
item, and its *Contract Changes Needed* table names the section of this document each amendment
lands in. Its nine owner decisions were ratified on 2026-08-10 — five as
`system_decomposition.md` D21–D25, four as calls inside the milestone — and two were refined on
2026-08-11.

**The M5 amendment wave is §22, and it moved the version to `contract-b/4.0`.** Each item below
now carries its disposition on its own first line: **Closed** with the amendments that closed
it, or **Open** with what it is still waiting for. **Items 2 and 8 stay open**, and §13 remains
the authoritative statement of what is open on this wire.

1. **CLOSED — §22, B22, B23, B32.** *No TLS, and one shared token* (§3.1). The wire is plain HTTP on a LAN, so a genome, a
   peer id and the token itself are readable in transit, and any token holder can present any
   `peerId`. M5 brings TLS and per-peer credentials together, because splitting them produces
   a half-secured relay that reads as secured. **Unchanged from M3, and now one milestone
   nearer.** **The version call is made — `contract-b/4`** (ratified 2026-08-10;
   `system_decomposition.md` D21, `m5_considerations.md` decision 1). Replacing §3.1's rule is
   not additive and there is an installed base, so the pair lands on a **major**, taken before
   strangers run the build. ~~Nothing in this section moves until that wave writes it~~ — **the
   wave wrote it**: §3.1's rule is replaced by a credential bound to the `peerId` (B22), the
   transport is TLS (B23), §3.2's `4006` requires the credential, and the version and path
   moved to `contract-b/4.0` and `/contract-b/v4` (B32).
2. **OPEN.** *A permanently rejected inbound organism is held, never returned* (§9.4). A safe
   two-phase return needs one more message pair. It stays parked while "held for an operator"
   remains an honest answer. **§22 did not touch it**, and the reason is unchanged: a public
   map makes "held for an operator" a *worse* answer, because the operator is now a stranger —
   but a two-phase return is a custody design and not a milestone's spare capacity, and M5 was
   already replacing the authentication model.
3. **CLOSED — §22, B29.** *Placement under churn is still only half-tested.* M4 places six known peers and splices
   one newcomer, by hand, once each. Strangers joining and leaving continuously is M5's
   problem, and it is where auto-placement, the coalescing window and `maxSlotEverIssued`
   growth will first be stressed. **B29 writes all three rules** — holes before growth on both
   claim routes, the shorter axis, a coalescing window that widens under churn and a repeat
   claim that broadcasts nothing. **What the rule does not do is test itself**:
   `maxSlotEverIssued` growth has still never been run at any scale, and WP5's synthetic churn
   harness is where it is measured before strangers are involved.
4. **CLOSED — §22, B27.** *The archive has no write interface and no authentication of its own.* It is a
   subscriber that trusts the shared token. A public relay cannot copy every envelope to
   whoever asks, so M5 needs a subscriber authorisation rule — and the M4 stats block makes
   that sharper, because a copied `PEER_STATUS` now carries every world's population and save
   state — since §16, the name of every species living in it (§6.3.1, B11), and since §19, its
   mod version, its save policy and the species it refuses to export (§6.3.1, B18). **B27 makes
   the subscribe grant a decision an operator takes** rather than a role a token holder
   declares, and it **states the visibility boundary** the two paragraphs above only implied:
   what a subscriber sees is exactly what every peer already sees, and it is a fairly complete
   profile of a stranger's machine, which is what D24's participant announcement has to say out
   loud.
5. **CLOSED — §22, B28.** *Release and handover are startup flags* (§7.5). If the map ever grows past what one
   operator can restart at will, both need an authenticated admin path — which is another
   reason they wait for the milestone that brings authentication. **The condition was met by
   the venue rather than by the size**: a hosted relay's restart drops every peer's session at
   once, so *at startup* is a price six innocent peers pay to fix one. B28 adds the path, adds
   `--evict-peer` beside the two, keeps the printed consequence report as a two-call
   confirmation, and keeps all three off the wire catalogue.
6. **CLOSED — §22, B26.** *The forwarding record does not survive a relay restart* (§5.2). Every outstanding
   `sent` entry loses its chance of a proof at that moment and falls back to the bounded hold
   (**and since §25's B37 there is no hold to fall back to: the entry is lost at
   `forwardTimeoutMs`**, which makes B26's receipt worth more than when it was written, not
   less).
   That is correct, and it is also a real cost the first time a relay is restarted under load:
   entries that could have been re-routed in seconds wait out a 24-hour clock instead. A
   durable record is not the fix — a record that outlives the process would assert what the
   process cannot know. The fix, if this ever hurts, is a **receipt**: the relay
   acknowledging each forward to the sender, which turns the sender's own journal into the
   evidence and costs one frame per migration. M4 does not buy that, because a relay restart
   is rare and `--release-inflight` is one command. **The receipt is proposed for promotion in
   M5** (`m5_considerations.md`, Design Question 2 and Contract Change 6): both halves of that
   last sentence are properties of a relay on the owner's own desk, and a hosted one has
   deploys, certificate rotation, kernel updates and a supervisor, while `--release-inflight`
   is typed on the sender's machine — which after M5 is usually a stranger's. The condition
   set here is unchanged; what changes is that the venue meets it. ~~A proposal, not an
   amendment: nothing in this item is normative yet.~~ **B26 made it normative**:
   `FORWARD_RECEIPT` (§6.12) is one frame per forward, it carries the `relaySessionId`, and it
   moves the fact into the sender's own journal. **It changes no safety rule** — §9.2's proof
   of non-delivery is untouched, a missing receipt is silence, and silence is still never
   proof. Its cost is one frame per migration on a relay whose virtue is that it forwards
   frames, and WP3 measures that at rate rather than assuming it.
7. **CLOSED for the wire's half — §22, B30.** *A stats block is unauthenticated telemetry from a peer.* The relay copies it verbatim
   into a broadcast every client reads. On a LAN of the owner's own machines that is fine; on
   a public relay a peer could report any population it liked, and the status page would show
   it. M5's per-peer credentials are the precondition for trusting any of it. **The census
   widens the surface without changing the argument** (§16, B11): a species name is
   attacker-chosen text of up to 64 bytes a slot, 32 entries a peer, that lands in a broadcast
   and then in a renderer. The wire's own answer is the shape check and the cap; the
   **renderer's** answer is its own, and a page that interpolates a name into HTML without
   escaping it has a defect this contract cannot fix for it. Named here rather than left to be
   discovered, because raw names are exactly the field an implementer assumes has been
   sanitized upstream — and A36 guarantees it has not. **§19's settings widen it once more and
   change the argument no further** (added — §19, B18): `migrationExclude` is another
   attacker-choosable string array on the same path to the same renderer, and `modVersion` and
   `contractAVersion` are two more free-text fields. Same answer — shape check on the wire,
   escaping in the renderer — with one addition specific to these: a reader **MUST NOT** parse
   `contractAVersion` into a capability decision, because a peer that can choose the string can
   choose the capability it claims. Detect a feature by the presence of its field
   (`contract-a.md` §3.1). **B30 moves both rules to §10.1, where the person implementing a page
   reads them**, states the escaping obligation as something a CI test asserts rather than
   something a contract asks for, names every attacker-chosen string on the page including the
   two this item never counted — `peerId` and the world name a player chose — and draws the
   moderation boundary: **suppression at the view, never removal from the record** (D11, §10).
   **The telemetry half of this item is now a precondition rather than a gap**: B22's credential
   means a stats block comes from a peer that is who it says it is. It still does not mean the
   *numbers* are true, and nothing on this wire will ever make them true — a peer can report any
   population it likes about its own world, and the page shows what it reported.
8. **OPEN.** *There is no control surface, and adding one is a design and not a field.* The operator
   surface is read-only end to end: every field on this wire flows peer → relay → subscriber,
   and nothing flows back toward a mod. The owner has ratified a control surface as **later
   work** (added — §19, B19). It needs its own message, and it needs answers this contract does
   not have yet — per-peer authentication (item 1), subscriber authorisation (item 4), an
   authenticated admin path (item 5), ordering and idempotency for a write that races a world
   load, and an audit trail. **Reversing `CONFIG_UPDATE` or making a stats field writable is
   not the cheap version of that work; it is the same work with the questions skipped**
   (`contract-a.md` §19, A43). **"Later" now has a milestone: M6** (ratified 2026-08-10;
   `system_decomposition.md` D23). ~~M5 supplies items 1, 4 and 5 — the three blockers — and
   still does not build the surface~~ — **§22 supplied all three** (amended — §22): the
   per-peer credential (B22), subscriber authorisation (B27) and an authenticated admin path
   (B28). **The surface is still not built, and that is the decision rather than an omission.**
   B28's path changes the relay's own registry — a reservation, an identity, a peer's admission
   — and touches nothing inside a world; the questions D23 defers are the ones about writing
   into a world, and none of them is answered by it. The accepted cost is stated as
   `m5_considerations.md` Risk 9: an operator can read a peer's `timeScale` (§18, B16)
   throughout the public release and can do nothing about it.

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

### B5 — A held entry keeps retrying, and that retry is the only conduit for the proof (§9.2, §9.3) — **SUPERSEDED — §25, B37**

> **Superseded in full on 2026-08-17 (§25, B37).** There is no `held` state and no retry. B5's
> analysis is still correct, and it is the reason §25 states a cost rather than claiming there
> is none: the retry WAS the only conduit a non-delivery proof had for an entry a previous
> process forwarded, so removing it makes `sent → refused → re-route` unreachable for exactly
> that case. B37 names it in its own table and the owner took it knowingly. **Nothing below is
> normative.** It is kept because a reader who proposes re-adding the retry should first read
> the argument that put it there.

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
| Tombstoned — delivered to the mod | `MIGRATION_ACK`, `duplicate: true` | Delivery is proven. Since B43, the ordinary durable ACK can still be pending. The immediate re-ACK can be the first one that reaches the relay (amended — §28, B43). |
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
| `MIGRATION_NACK` the relay **generates in answer to a `MIGRATION_PAYLOAD`** — `SLOT_VACANT`, `PEER_OFFLINE`, `NOT_FORWARDED` | **yes** | Proof-bearing answers carry `neverForwarded` and `relaySessionId` (§5.2, §6.8). A 4.2 queue refusal also carries `refusedAttempt`. Graceful-drain `NOT_FORWARDED` is proof-free. Each answer records why the relay declined that attempt (amended — §31, B46). |
| `NOT_A_MEMBER` — a payload from a subscriber | no | A **role** error about a connection. No migration was ever in question, and the offending frame was not forwarded to anybody. |
| `PEER_UNKNOWN` — a routed `ACK`/`NACK` whose `destPeer` has gone | no | A **routing** error about a frame the archive already has a copy of, from the fan-out of the original. |

**The archive's legacy NACK key is `migrationId` + `code`, not `migrationId` alone.** One
migration can produce several different refusals. A field-present 4.2 queue refusal also adds
its refused destination and reroute count to the key (amended — §31, B46). Thus, two queue
refusals in one bounded walk remain separate facts. A proof-free drain and every older NACK keep
the legacy key. A `MIGRATION_PAYLOAD` and a `MIGRATION_ACK` still deduplicate on `migrationId`.

**The key must be built in exactly one place**, and writing this amendment is what found the
reason. The archive had two: the live path composed `migrationId` + `code`, while the restart
replay rebuilt the seen-set from disk under `migrationId` alone, for every record type. The
two keys could never match, so a NACK re-copied after a restart was recorded a second time —
the §5.1 guarantee held within a process and not across one. A duplicate *row*, never a
duplicate organism, and invisible until a restart happened to coincide with a re-forward. The
reconciliation pass collapsed both call sites onto one key function.

**Enforced by:** the relay, for the fan-out; the archive, for the key — and for keeping the
key single-sourced, which is the part that broke.

### B8 — Dedup precedes the decode, and a live destination's retry backs off (§6.6, §9.2, §9.3, §9.4, §12) — **SENDER HALF SUPERSEDED — §25, B37**

> **The RECEIVER half stands and is more load-bearing than ever** (§25, B37): dedup before the
> decode is what makes a pre-B37 sidecar's retry cost the destination an O(1) lookup instead of
> a multi-MiB unmarshal, and those senders are the installed base this map keeps serving. **The
> SENDER half is superseded in full**: there is no re-forward to back off, `forwardRetryMaxMs`
> is removed from §12, and the hammer B8 bounded cannot come back, because the code that swung
> it is gone.

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
and already counts per-lane offers (§10.1). It now also keeps a **bounded, in-memory feed of
recent, receiver-acknowledged deliveries** — lane, species names, timestamp — and serves it
beside the status view, so the page can animate that species' glyph travelling along the lane's
arrow. **Corrected 2026-08-23:** a copied payload alone is not a delivery; population admission
made that previously hidden distinction visible.

**Resolution.**

| Rule | Statement |
|---|---|
| Source | The `MIGRATION_PAYLOAD` and `MIGRATION_ACK` copies the archive already receives as a subscriber (§5.1), correlated by `migrationId` and the destination slot's peer binding. The ACK exists only after Contract A's `MIGRATE_IN_ACK` (§6.7), so it proves the destination game accepted the spawn. **§10.1's first rule is unchanged and unweakened**: one subscribed stream, no active polling of worlds, and nothing on the migration path ever waits for a reader. |
| Bounded twice | By **time** (~60 s) **and** by **count**. Both, not either. The status view is serialized verbatim into the durable metrics file once a minute, so an unbounded array would be written to disk every minute forever. |
| It is delivery, not census | The feed answers *what a receiver acknowledged spawning*, and B12's rules apply unchanged: it is labelled as history, it is **never summed** into an abundance claim, and it is not joined to the census without normalizing for the comparison only (`contract-a.md` §16 A34 versus §17 A36). A `MIGRATION_NACK` removes the matching pending attempt; a reroute with the same `migrationId` replaces its attempted destination; only the ACK from that destination emits one hop. |
| An absent species block | Renders as the **neutral glyph** — never a guessed name, never omitted, never "unknown" as a species *value*. That is §10.1's unknown rule applied to a new view rather than bent for it. |
| Names are still untrusted text | §13 item 7's escaping obligation applies, and it applies to a name that now reaches a **new** part of the page. A census name and a ledger name are equally untrusted. |
| Not a wire feature | No sidecar, relay or peer learns that the feed exists. If the endpoint is unavailable, the page keeps its map and aggregate numbers but pauses delivery animation; it may not turn payload-derived counter changes into arrival flashes. |

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
| The ambient lane pulse is removed | The map no longer walks a decorative dot down each lane at that lane's measured rate. The payload-derived offer rate stays as the lane's own numeric label, which is what a reader can actually compare. **What moves on the map is now always a delivery the receiving game acknowledged** (§17, B14) — travelling under motion, still-and-fading under reduced motion, and the destination cell's arrival flash either way. Rejected offers never move. |
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

---

## 19. The world settings readout (`contract-b/3.5`, 2026-08-07)

The owner ratified **the Species and Settings tabs on the site** on 2026-08-07, against the
running M4 deployment. The species tab is already served: §16 put the census on this block and
§10.1 states what the page may claim from it. **The settings tab has no inputs at all.**

`contract-a.md` §19 puts five of them on the mod's handshake. This set carries those five the
rest of the way, and adds two the sidecar has always held and never published: the peer's
`modVersion` and the `contract-a` identifier its mod session is speaking.

**Two amendments, B18 and B19**, continuing the `B` series for the reason §14 gives. B18 puts
the seven fields on the wire; B19 states what the page does with them, records the read-only
boundary, and applies §4's version test.

**This set changes the wire, additively**: seven OPTIONAL fields on one existing object. No
message type, no field removal, no type change, no enum value added or removed, no new NACK
code, and no change to custody, dedup, the hold, the fan-out, hashing, routing or admission
control. §4's test therefore answers **minor**, and the identifier moves to `contract-b/3.5`
(B19). `contract-a.md` takes a **minor** of its own, to `contract-a/2.3`, because five of the
seven are new fields on that wire (§19, A44) — the first time since §15 that both contracts
move together.

### B18 — The stats block carries the world's mod settings and versions (§6.3.1, §6.5, §6.11, §10.1)

**Gap.** Every field this block has carried since M4 answers *what is this world doing* —
population, custody depth, paced depth, held depth, simulated time, speed, the last save, the
species alive. **Nothing answers *what was this world told to do*.** That is the missing half
of three readings the page already shows:

| The page already shows | It cannot currently say |
|---|---|
| A lane with no traffic | whether that world has no migrants, or whether its whole population is on an exclusion list. On the shipped default this is the **common** case: `Basic bibite` is a large resident population in every seeded world and zero percent of every lane's traffic (`contract-a.md` §18, A39). |
| A slot with no `lastSave` | whether that world has not saved **yet**, or whether its save timer is **off**. Those have opposite consequences for a hard stop, which is the failure D14 exists for. |
| A slot with no census, or no cap, or no speed | whether that peer's mod is too old to send one, or whether something is wrong. `contractAVersion` turns every one of those puzzles into a fact. |

The last row is why the two version strings are in this set rather than a later one. This
document has now added four field groups in three days — the species block, the census, the
pacing settings, these — and each added a slot state that reads as "unknown" for a reason no
reader can see. **Publishing the negotiated `contract-a` version makes "unknown" self-explaining
for every field group, including the ones added after this amendment.**

**Resolution.**

| Rule | Statement |
|---|---|
| The five mod settings | `migrationExclude`, `saveMinutes`, `saveKeep`, `saveOnQuit`, `worldWrapping`, copied **verbatim** from the last `CONFIG_UPDATE` the sidecar received (`contract-a.md` §5.1, §19 A42). The sidecar does not author, compute, default, repair, re-normalize or infer any of them, exactly as it does not author a census (§16, B11). |
| `modVersion` | Copied from `CONFIG_UPDATE.modVersion`, which this wire's sidecar has held since M2 and never forwarded. **Not a capability statement**: what a mod can do is `contractAVersion` plus, field by field, presence (`contract-a.md` §3.1). A reader that gates a rendering on a version *string* rather than on a field's presence is doing the arithmetic §3.1 forbids, with a looser number. |
| `contractAVersion` | The `protocol` identifier on that mod's frames — the session's own, not the sidecar's build (`contract-a.md` §3). A sidecar **MUST** publish what the peer actually sent, never what it supports: those differ on exactly the rig this field exists to describe. |
| Static, and cheap because of it | All seven change only when a mod reconnects or sends `reason: "settings_changed"`. The sidecar holds the last values and re-sends them with each stats block it was already sending. No new cadence, no new trigger, no new message. |
| Absence | **Unknown**, in every case: no mod is connected, the peer's build predates this amendment, or that mod predates `contract-a/2.3`. A reader **MUST NOT** substitute a default for any of them (§10.1). |
| The two readings that are not gaps | `saveMinutes: 0` is a save timer that is **off**, and a present `migrationExclude: []` is a policy that is **off**. Both are facts, both explain something else the page shows, and both are destroyed by a reader that folds them into absence. This is `timeScale: 0`'s rule (§18, B16) applied twice more. |
| The two name rules stay apart | `migrationExclude` entries are **A34-normalized** (`contract-a.md` §16, A34) because that lane matches; `species[].genericName` and `.specificName` are **raw** (`contract-a.md` §17, A36) because that lane labels. Both live on this one block, and no party may apply either rule to the other's field. A consumer that wants to know whether an excluded species is in the census normalizes the **census copy for the comparison only**, and rewrites neither. |
| The relay | **Unchanged, and for the third time that is the point.** §16's B11 asked a relay to store the stats block as the bytes it arrived as, "for the next field after this one". §18's three were that field; these seven are the next. A `contract-b/3.4` relay carries all seven without knowing they exist. |
| Not a routing input | Seven more values copied into a broadcast the relay was already sending. Nothing routes, schedules, refuses or filters on any of them — and **the exclusion list in particular is not enforceable by any party that now sees it** (`contract-a.md` §18, A39; §19, A42). |
| Read-only, one way | There is no path by which any of these seven travels back toward a mod. §6.3.1 states it as a rule and `contract-a.md` §19, A43 states why a control surface is a separate design rather than a reversal of this one. |
| Size | Two short strings, three numbers, a boolean and a short array — under 200 bytes on a block whose bound is the census (§6.3.1). |

**Enforced by:** the **sidecar**, for copying the mod's settings rather than authoring them and
for publishing the peer's negotiated version rather than its own; the **relay**, for carrying
what it does not understand; the **archive and any client**, for rendering an absent value as
unknown and for never sending one back.

### B19 — The page serves a settings view, it is read-only, and the version moves to `contract-b/3.5` (§4, §6.3.1, §10.1, §12)

**Change.** Three things, and the third is a boundary rather than a feature.

**Resolution.**

| Rule | Statement |
|---|---|
| Settings per world | Each world's card may state its mod version, its negotiated `contract-a` version, its exclusion list, its save policy and its wrap. §10.1's new row binds all of it. |
| The cause beside the effect | The value of this view is that it sits next to the numbers it explains: the exclusion list beside the census that contains the excluded species and the lane that never carries it; `saveMinutes` beside `lastSave`; `contractAVersion` beside whichever field group that mod is too old to send. A settings tab rendered somewhere a reader cannot see those numbers is a list of values with nothing to answer. |
| Unknown, out loud, and one default named | §10.1's unknown rule with no exception, and one substitution called out by name because it is the tempting one: **`saveMinutes` must never render as `10` because that is what the mod ships with.** A page that does it claims a world is being saved when its timer may be off. |
| Read-only, and stated on the page | The view offers no control. A future control surface is owner-ratified as **later work** and is a separate design — its own message, its own authentication, its own authorization across two machines, its own ordering and its own audit trail — none of which is an extension of an OPTIONAL field on a stats block (`contract-a.md` §19, A43). |
| Still a page decision | No field, message, component or wire behaviour is added or removed by any of it. It is recorded here because §10.1 is where what the page may claim is written down, and "this world never exports `Basic bibite`" is a claim. |

**Version.** Apply §4's test:

| Change to Contract B | Kind | Needs a major? |
|---|---|---|
| Stats block gains `modVersion`, `contractAVersion` | additive OPTIONAL fields | no |
| Stats block gains `migrationExclude`, `saveMinutes`, `saveKeep`, `saveOnQuit`, `worldWrapping` | additive OPTIONAL fields | no |
| §10.1 gains the settings rule | a page-input rule, off the wire | no |
| §6.3.1 gains the read-only rule | a prohibition on behaviour that does not exist | no |
| Message catalogue, enums, codes, custody, dedup, routing inputs, fan-out, hashing, the hold | **all unchanged** | no |

The identifier is **`contract-b/3.5`**. Every `contract-b/3.x` peer stays compatible with every
other, because compatibility is on the major and the minor is never a rejection reason (§4,
`contract-a.md` §3.1).

**What a mixed rig does, honestly.** The degradation is per slot and, for the first time, it
**names itself**:

1. A `contract-b/3.5` **sidecar** with a `contract-a/2.3` **mod** publishes all seven. This is
   the target configuration.
2. A `contract-b/3.5` **sidecar** with an older **mod** publishes `modVersion` and
   `contractAVersion` — it has always had those — and no settings. The card reads *"mod 0.6.0,
   contract-a/2.2 — settings not reported by this mod"*, which is the first unknown on this page
   that explains itself.
3. A `contract-b/3.4` **sidecar** publishes none of the seven, and its card reads unknown
   throughout while its population, census, depths and pacing stay exact.
4. A **relay** that re-encodes the block from a typed model instead of carrying it drops all
   seven, for every slot. §16's B11 wrote that SHOULD down before there was a field that needed
   it, and this is the third field group it has protected.
5. No path produces a wrong setting, because there is no default anywhere in the chain to
   produce one from — and no path produces a **changed** setting, because nothing in this system
   writes one.

**Enforced by:** the **page and `ringstat`**, for rendering unknown rather than a default, for
keeping `saveMinutes: 0` and `migrationExclude: []` distinct from absence, and for offering no
control; the **archive**, for carrying the seven fields through `StatusView` untouched; the
**sidecar**, for copying rather than authoring; both wire ends, for sending
`"contract-b/3.5"` and comparing only the major.

## 20. The disk budget (2026-08-08)

The living deployment ran out of root filesystem overnight on **2026-08-08**, and
nothing in this contract had ever said what a peer is allowed to consume. Every
durable file it names is append-only; two of them shrank at no point a running
process could reach. Five sidecars that had been up for two days held 445 MB,
500 MB, 905 MB, 683 MB and 720 MB of journal for a live set of a few hundred
entries, beside 3.5 GB of unrotated log, on a 98 GB volume shared with other
work.

**One amendment, B20**, continuing the `B` series for the reason §14 gives.

**This set does not change the wire.** No message type, no field, no enum value,
no NACK code, no change to custody, dedup, the hold, the fan-out, hashing,
routing or admission control — and no change to what any peer sends or accepts.
§4's test therefore answers **neither major nor minor**, and the identifier stays
at **`contract-b/3.5`**. A peer running the fix and a peer without it are
indistinguishable on the wire, which is the point: this is a statement about what
a peer does to its own disk.

### B20 — A peer bounds its own journal and its own log (§10, §12, `contract-a.md` §11.1)

**Gap.** `contract-a.md` §11.1 required the journal to be durable and §12 sized its
retention in *time*, and neither ever bounded it in *bytes*. The compaction that made the
journal small existed only inside `Open`, so the bound was "restart the process",
which is not a bound — a rig whose whole value is that it stays up cannot have a
disk budget that depends on it going down. The log had less than that: it was
whatever the operator's shell redirect caught.

**Resolution.**

| Rule | Statement |
|---|---|
| The journal compacts on a timer | A sidecar MUST rewrite its journal to the entries it still holds at least every `journalCompactMinutes`, not only at `Open`. The rewrite uses the same crash-safe discipline `contract-a.md` §11.1 already requires — scratch file, fsync, rename, directory sync — so a crash before the rename leaves the original whole and a crash after it leaves a journal that replays to the identical state. |
| The deadline survives it | A compaction MUST preserve `sentAt` on every `sent` entry (§9.3, amended — §31 B46). `forwardTimeoutMs` is measured from it, so restart cannot move an unresolved attempt's deadline. A proven refusal or reroute clears the old value. A pending or refused entry MUST NOT replay the prior attempt's loss deadline. |
| The purge record stays | A completed inbound entry with `ackedUpstream: false` is not expired and is not purgeable (amended — §28, B43; §6.7). For every eligible tombstone, `PurgeExpired` still appends a durable purge record even though the next compaction would erase it. `contract-a.md` §11.1 makes every journal write durable before it counts and keeps tombstones out of memory-only state. The saving does not justify an exception. A purge record is one short line per eligible tombstone. The growth term was each migration's payload-bearing `create` record. |
| A process may own its log | A peer MAY be given the path of its own log file, and when it is, it MUST bound it: rotate at `logRotateMb` and keep `logKeep` generations, so its ceiling is `logRotateMb × (logKeep + 1)`. Rotation MUST fall between two records and never inside one. Given no path, it logs to a caller-supplied stream and bounds nothing, which is the pre-M4 behaviour and stays the default for tests and interactive runs. |
| A failed write cleans up after itself | Every rename-into-place in this system — the genome store's, the journal's, the relay's map — MUST remove its scratch file on the error path. The store's sweep MUST also collect scratch files old enough that no live write can own them, because a process killed between the write and the rename cannot run its own cleanup. |
| A failed append leaves no bytes behind, and a replay says what it could not read (the ledger's half added 2026-08-09) | Every append-only file here — a sidecar's journal, the archive's `migrations.jsonl` — MUST truncate back to its pre-write length when an append fails or lands **short**, so no fragment is left for the next append to splice a whole record onto, and MUST drop an unterminated final line before appending again. A replay MUST report what it could not read rather than ending in silence. Where the file is rewritten as a matter of course — the journal, which compacts at `Open` — a replay MAY stop at the damage and report the history it discarded behind it. Where the file is **never** rewritten — the archive's ledger, whose contents nothing may evict (§10) — a replay MUST **skip** the damaged line and keep every record behind it, because the damage is permanent and stopping would make the loss grow with the file, without bound, forever. **Segmentation does not weaken that argument and this row's reasoning is restated rather than assumed** (amended — §26, B40): a segment is closed once and never rewritten, so damage inside one is exactly as permanent as damage in the single file was, for the life of that segment. What §26 changes is only that the damage now leaves the host with its segment instead of being read past forever, and that the aggregate the replay feeds (§26, B39) counts the line it could not read rather than losing the fact. |
| What stays unbounded is named | The archive's `migrations.jsonl` and its genome store are the record of what happened and nothing evicts from them (§10). They grow with **traffic**, not with uptime. `metrics.jsonl` grows with **time**, at one sample per `metricsInterval`. An operator sizes a disk for these three; no peer will ever reclaim them. **Two of the three still are, and the genome store is now the operator's choice** (amended — §23, B33): `migrations.jsonl` and `metrics.jsonl` are unbounded exactly as this row says, and the **genome store** grows without bound only while `genomeRetentionHorizon` is unset — which is its default, so this row remains true of every deployment that has not decided otherwise. Where a horizon **is** set, the store's steady state is the horizon's worth of blobs and the sizing question becomes a rate rather than a total. ~~Nothing changes for the ledger, at any setting.~~ **One of the three is left, and it is `metrics.jsonl`** (amended — §26, B39/B40). The ledger's per-crossing lines are bounded by `ledgerWindow` (§12) when a deployment sets one, and their steady state is likewise a rate rather than a total; the **aggregate** they fold into is kept forever and is bounded by construction, because its size follows the number of species, lanes, peers and buckets and not the number of crossings. **What the operator now sizes for is a window plus a fold, not a file that only grows.** Unset, this row still reads exactly as it was written, which is the default. |

**What the outage cost, as evidence for *A failed write cleans up after itself*.** When the volume filled,
every genome write in the rig failed inside `os.WriteFile` — after the file
existed and before it held anything — and left an empty `<hash>.json.tmp`
behind. The store is content-addressed, so the same genome retried under the same
name and left the same corpse again: **15,119 zero-byte files**, every one an
inode spent on nothing at the moment inodes and blocks were what the rig had run
out of. A cleanup on the error path costs one syscall on a path already failing.

**Enforced by:** the **sidecar**, for scheduling the compaction outside its
custody lock and for treating a failed reopen after the rename as a reason to
stop journaling rather than to keep appending to an unlinked file; the
**journal**, for the rewrite's crash safety, for carrying a sent attempt's `sentAt`, and for
preserving its durable clearing on refusal or reroute; the **archive**, for the same all-or-nothing append on its ledger and
for reading past a damaged line rather than stopping at one it can never rewrite;
**every process**, for bounding a log it was given the path of; the
**genome store**, for its error path and its sweep; and the **operator**, for
sizing a disk against a ledger that no rule in this document will ever shrink
(amended — §26, B40: **§26 is that rule**, and what the operator now sizes for is
the window they set, the fold behind it, and the segments still waiting for an
off-host copy).

## 21. The fetch queue's own bounds (2026-08-10)

On the evening of **2026-08-10** the living deployment's archive spent thirty minutes
unable to hold a relay session: **26 drops and 26 reconnects between 20:22:52Z and
20:52:23Z**, absent for 12% of that span, and **7,789 crossings** — 23% of everything that
crossed while it ran — are permanently missing from the ledger as a result. Nothing was
wrong with the wire, the map or the traffic: `timeoutBounces` stayed 0, no sidecar dropped,
no mod took a `4004`, and 24 of 24 lanes read `peer_live` throughout. What was wrong was
that §10 gave the genome fetcher a **rate** and never gave it a **budget**.

`genomeRequestsPerMinute` bounds how many requests one requester may send one peer per
minute. It says nothing about how much work a single pass of the queue may do, and nothing
about how many of that minute's requests may leave at once. At a backlog of 64,736 gaps —
which the ×100 regime reaches in a day, because the crossing rate no longer falls low enough
for the queue to drain — both of those unstated numbers were the whole problem. The archive's
pass walked all 64,736 entries under the one lock its frame handler also needs, `stat`ing the
genome store once per entry: **0.3 to 1.0 s of held lock per one-second pass**, measured, and
more under host load. Its read loop was admitted about once per pass instead of the ~30
frames a second the relay was copying to it, the relay's per-subscriber outbound queue filled
in about four seconds, and **the relay closed the session with `1011 "outbound queue full"`**.
The resubscribe changed nothing about the backlog, so it happened again, and again.

**One amendment, B21**, continuing the `B` series for the reason §14 gives.

**This set does not change the wire.** No message type, no field, no enum value, no NACK
code, no change to custody, dedup, the hold, the fan-out, hashing, routing or admission
control — and no change to what any peer sends or accepts. A requester running the bound
still never exceeds `genomeRequestsPerMinute` to any peer, still asks the source first and
then the ring in structural order, and still retries on §10's ladder at the same moments;
it merely spreads the same requests over the minute instead of emptying the allowance into
one millisecond. §4's test therefore answers **neither major nor minor**, and the identifier
stays at **`contract-b/3.5`**. This is a statement about what a requester does to its own
read loop.

### B21 — A rate is not a budget: the fetch queue bounds its own work, its burst and its concurrency (§10)

**Gap.** §10's *Fetch behaviour, archive side* table named a rate limit, a retry ladder and
"keep the hash forever", and between them they describe a queue that may grow without bound
while the cost of one pass over it grows with its size. Nothing in this document said that a
requester must not spend an unbounded amount of time in one pass, must not hold its own lock
across the whole of one, or must not put a whole rate window on the wire in one burst. On a
small backlog none of that is visible: with 60 gaps the pass is free, and the flat zeros of
2026-08-07 hid the shape completely. **The rule was missing for as long as the queue was
small, which is exactly how long nobody could have measured it.**

**Resolution.**

| Rule | Statement |
|---|---|
| A pass is bounded in work, not in backlog | A requester MUST examine at most `genomeScanPerTick` pending gaps in one pass of its fetch queue. The cost of a pass is then a property of the requester and not of how far behind it is, which is the only form in which that cost is bounded at all. |
| The whole backlog is still visited, and in a fixed order | The walk MUST be round-robin over a **stable** order and MUST resume where the previous pass stopped, so a full cycle takes `ceil(len(pending)/genomeScanPerTick)` passes and **no gap can be starved**. A budgeted walk over a hash map is not enough: map iteration order is unspecified, and a bounded prefix of a random order leaves a hash unexamined for an unbounded number of passes. |
| A pass MUST yield, and this is the rule the incident is about | A requester MUST NOT hold, across a whole pass, any lock that its own read loop or heartbeat needs. It MUST release and retake that lock at least every `genomeScanChunkSize` gaps. **A subscriber that stops reading is indistinguishable from a subscriber that is gone**, and the relay will treat it as one: the peer's outbound queue fills and the relay closes the session (§5.1, `archiveQueueMax`; `contract-a.md` §11.1's "close rather than grow without limit"). Risk 4 says nothing on the migration path may wait for a reader — it does not say the reader may stop reading. |
| The burst is bounded independently of the rate | A requester MUST send at most `genomeRequestsPerTick` `GENOME_REQUEST`s in one pass. `genomeRequestsPerMinute` admits a whole window's worth the instant the window opens; a pass over a large backlog therefore emptied the allowance for **every** peer inside a few milliseconds and then sent nothing for the rest of the minute. The same requests, spread, cost the same rate and no burst. |
| Concurrency is bounded independently of the rate | A requester MUST NOT carry more than `genomeInFlightPerPeer` unanswered requests to one peer. Both new bounds are sized **above** what `genomeRequestsPerMinute` can sustain, so neither ever lowers the fetch rate; they bound the shape of the traffic, not its volume. |
| A resubscribe inherits no in-flight requests, and no ladder change either | A request sent on a session that has ended can never be answered, so on a new session a requester MUST forget its per-peer in-flight **accounting** — otherwise a flap leaves the new session's concurrency budget already spent and the fetch stalls for a whole `genomeRequestTimeoutMs`. It MUST NOT touch anything else: attempts, `nextAt` and the request deadline stay exactly as they were, so **the ladder retries at the moment it always would have**. That is the line this amendment does not cross — it changes how many gaps are asked per pass and never when a gap is due. |
| A timeout falls due on time, not when the cursor comes round | Because the walk is now round-robin and budgeted, a requester MUST reap expired requests from the set of outstanding ones rather than only when the walk reaches that entry. That set is bounded by `genomeInFlightPerPeer` per peer, so the reap costs the same at 60 gaps and at 60,000 — and `genomeRequestTimeoutMs` keeps meaning what §10 says it means. |
| A send failure ends the pass | When a send fails, a requester MUST stop the pass rather than walk on. Continuing put nothing on the wire and logged one failure per remaining entry: 231 lines in one instance of this incident, which is noise on exactly the log an operator is reading to understand a flap. |

**What it measured.** The archive's fix was deployed to the living deployment at
**21:08:50Z**, 89 seconds of replay, and read as follows against the same hour before it:

| | Before | After |
|---|---|---|
| Requests in the widest one-second window | **180**, inside 33–176 ms | **8** |
| Fetch rate achieved | ~175/min | ~175/min — unchanged, because the rate cap still binds |
| Longest gap in the recorded-migration stream | **6.5 s** | no session-ending gap; zero drops |
| Archive process CPU, steady state | **~100%** of a core | **8.1%** |
| Relay sessions lost in 30 minutes | **26** | **0** |

**The CPU line is the one to read twice.** 64,736 `stat` calls a second is a whole core spent
on a question the requester had already answered — a hash is in `pending` precisely because
the store does not hold it — and the read loop was queued behind every one of them. The
bound did not make the fetcher slower; it stopped it doing 97% of a pass's work for nothing.

**Enforced by:** the **archive**, as the only requester in M4, for the round-robin cursor,
the chunked lock, the per-pass and per-peer bounds and the session-scoped in-flight
accounting; the **relay**, unchanged, for going on closing a subscriber that stops reading,
which is the behaviour that made this visible rather than silent; and the **operator**, for
reading `genomeGaps` as a queue whose *magnitude* now has consequences and not only as a
number on a page.

## 22. The public-release amendments (`contract-b/4.0`, 2026-08-11)

M5 takes this wire out of the owner's house. Every rule in this document up to §21 was
written for a network of two computers one person owns, and §13 said so item by item: no TLS,
one shared token, an archive that trusts it, operator commands that are startup flags, and no
limit of any kind on what a peer may do to the relay. **A public relay meets peers nobody
vetted**, and every one of those items becomes a defect the moment it does.

The nine M5 decisions were ratified on **2026-08-10** and two were refined by the owner on
**2026-08-11** (`system_decomposition.md` D21–D25; `m5_considerations.md`, *Decisions for the
Owner*). This set is the wire they describe. It is written **before any M5 code**, which is
WP1's whole reason to exist: `contract-b/4.0` is what every other M5 package builds against,
and M2 and M3 both paid for the alternative.

**Eleven amendments, B22 to B32**, continuing the `B` series for the reason §14 gives, and
each one carries the row of `m5_considerations.md`'s *Contract Changes Needed* it comes from:

| Amendment | Source row | What it does |
|---|---|---|
| **B22** | row 1 | Per-peer credentials replace the shared token, bound to the `peerId` |
| **B23** | row 2 | TLS: the scheme, the certificate, the rotation |
| **B24** | row 9 | Capacity limits as a published table, every one a knob |
| **B25** | row 10, additive half | A minimum **contract** version at the handshake — a compatibility gate, never a security control |
| **B26** | row 6 | The relay forward receipt |
| **B27** | row 4 | The archive as an **authorised** subscriber, and what it may see |
| **B28** | row 5 | An authenticated admin path for release, handover and eviction |
| **B29** | row 8 | Auto-placement under churn, and the broadcast bound |
| **B30** | row 7 | The renderer's escaping obligation as a testable rule, and the version prohibition where a page implementer reads it |
| **B31** | row 10a | The envelope's game version as diagnostic metadata, D22's layered statement, and the four kept gates |
| **B32** | row 15 | `contract-b/4.0`, `/contract-b/v4`, and the migration note for the living deployment |

**Rows 3, 11, 12 and 13 are Contract A's**, and `contract-a.md` **§21, A47–A52** — authored in
the same wave — carries them at `contract-a/2.4`: **A47** the bearer token on that wire's
upgrade (row 3), **A49** the `parents[].blobDroppedForSize` flag that finally makes
`"blob_dropped_for_size"` reachable (row 11, §6.6 here), **A50** the sidecar's refusal of a
declared export set the map cannot use (row 12), **A51** the fourth assessment of contract debt
A5, which stays open and moot (row 13), and **A52** the version call (row 15). **Row 10a is
shared and each document writes its own half**: B31 below states D22's layering and the four
kept gates for the map, and `contract-a.md` §21, A48 states them for the mod and the sidecar.
**Row 14 is not written**: decision 9 took
the velocity floor out of the milestone, to be instrumented during the playtest and built only
if the measurement says so, and `contract-a.md` §12 item 7 stays open across a public release
(`m5_considerations.md`, Risk 8). **Row 16 is a documentation correction in other files** and
is not a contract change.

**This set changes the wire, and one row of one table is why it is a major.** §3.1's
shared-token rule is **replaced** and it has an installed base — the shape §3.1's own test and
`contract-a.md` §15's A41 both put on the expensive side. Everything else here passes the
additive test: **one** new message type (`FORWARD_RECEIPT`), one new section (§3.3), one new
close code (`4007`), one new `SECTOR_GRANT` reason (`"rate_limited"`), new OPTIONAL fields on
existing objects, and rules written down where none existed. **The message catalogue, the
envelope, custody, dedup, the hold, the fan-out, hashing and the routing inputs are
untouched.** §4's test therefore answers **major**, and the identifier moves to
**`contract-b/4.0`** with the URL path to **`/contract-b/v4`** (B32).

Affected body text carries an `(amended — §22, Bx)` or `(added — §22, Bx)` marker, and
**§22 wins over the body and over §14 to §21 wherever they disagree.**

**One thing this set deliberately does not do, and it is the exception that shapes B31.**
**§22 retires no version gate.** The owner was asked on 2026-08-11 whether D22's layering
should retire the four shipped mechanisms that decide on the game version, and answered
*"lets not change this then we will reconsider in the future if there's issues i think we can
leave it like its working now."* So §6.1's relay refusal stands, §9.2's `VERSION_UNSUPPORTED`
stands, §8's `peer_incompatible` skip stands, and `contract-a.md`'s close `4002` stands. B31
writes them down as **kept, deliberate exceptions** and states what they cost, which is more
useful than a prohibition four shipped mechanisms violate.

### B22 — The shared token is replaced by a per-peer credential bound to the `peerId` (§1, §3, §3.1, §3.2, §6.1, §7.5, §12, §13 item 1)

*Contract Change 1 (D21). **This is the breaking change**, and it is the reason
`contract-b/4.0` exists.*

**Gap.** §3.1 gave the whole map one bearer token and said plainly what it did not buy: *"it
does not authenticate a peer identity: a token holder can present any `peerId`, including one
that already holds a slot, and the `4006` rule will then evict the legitimate peer."* On a LAN
of the owner's own machines that composition is fine, and §3.1 says why. **On a public relay it
is a one-frame denial of service against any peer whose `peerId` can be read off the status
page** — and the status page publishes every one of them.

**Resolution.** §3.1 is rewritten; the table there is normative and this is what it settles.

| Rule | Statement |
|---|---|
| One credential per peer | `Authorization: Bearer <peerId>.<secret>` on the HTTP upgrade, split on the **last** `.` because a `peerId` may legally contain one. Never in a frame — a frame is logged, copied to subscribers and forwarded. |
| **The binding is the security property** | The relay MUST verify that the credential's `peerId` equals the `HANDSHAKE.peerId`, and MUST refuse the connection when they differ. **A valid credential for `peer-A` presented with `peerId: "peer-B"` is refused at the handshake, and `peer-B` observes nothing**: no close, no `4006`, no `PEER_STATUS` change, no `lastRefusal`. §6.1 carries the worked example, and this sentence is the acceptance test WP2 reports against (Risk 1, D21). |
| `4006` requires it | §3.2's eviction now fires only for a connection that **authenticated as** the same `peerId`. The self-healing rule `contract-a.md` §2 gives the mod socket survives; the impersonation it permitted does not. |
| A refused peer waits for a person | HTTP 401, the backoff ladder, the ceiling at `authFailuresBeforeCeiling`, and a log line that names **the remedy and who must act**. A sidecar whose credential is refused MUST NOT generate a fresh `peerId`, fall back to an unauthenticated connection, fall back to `ws://`, or try another peer's credential. It keeps its journal and goes on delivering inbound entries to its own mod. |
| Issuance is the smallest design that works | The relay mints the secret at first claim and prints a **join string** — relay URL, `peerId`, secret — once, at its own console, for an operator to hand over out of band. **No accounts, no email, no password reset** (DQ1). |
| Recovery is a slot handover | §7.5's `--handover-slot`, over B28's authenticated path, rebinds the reservation to a new `peerId` with a freshly minted credential. It already exists, already refuses to run while the old peer is live, and already prints its consequence. |
| Storage is a verifier | The relay keeps a salted hash and the grant, in `credentialVerifierStore`, never the secret. A relay whose store is read must not thereby hand over every peer's identity. |
| Grants are disjoint | **peer**, **subscribe** (B27) and **admin** (B28). One credential, three possible grants, and a credential that holds one does not hold another. |

**The cost, stated because it is real and was chosen with the price in view.** There is no
recovery path in the software. **A stranger who loses their join string loses that world's
identity until an operator hands the slot over by name.** That is the honest price of *no
accounts*: the alternative is an account system, an email, a reset flow and a support surface,
in the milestone that already replaces the authentication model, hosts the relay and meets
strangers (DQ1). The cost is bounded by the thing that makes it recoverable at all — the
reservation never expires, so the slot, the position and every journal entry addressed to it
are still there when the operator gets to it.

**What it does not buy, said so nobody assumes it.** A credential authenticates *who is
speaking*, not *what they say*. A peer can still report any population, census or setting it
likes about its own world (§13 item 7), and no rule on this wire will ever make a stat true.
What changes is that the stat is now attributable.

**Enforced by:** the **relay**, for the binding, the constant-time comparison, the verifier
store and the mint-at-first-claim; the **sidecar** and the **archive**, for holding their own
credential, for the 401 discipline and for never inventing a new identity to get past a
refusal; the **operator**, for handing over join strings and for never being asked to type
`--insecure-no-token` by any document this project ships (`m5_considerations.md`, decision 7).

### B23 — TLS at the relay's front door (§3, §12)

*Contract Change 2.*

**Gap.** §3's transport table said *"WebSocket over plain HTTP. TLS is M5"*, and §13 item 1
named the reason the two halves ship together: **splitting them produces a half-secured relay
that reads as secured.** TLS without credentials encrypts a wire on which any participant can
impersonate any other; credentials without TLS put the credential on the wire in the clear.
Neither half is a milestone; the pair is.

**Resolution.**

| Rule | Statement |
|---|---|
| The scheme | Clients dial **`wss://`**. A public relay MUST refuse a plain `ws://` upgrade — HTTP **426** with `Upgrade: TLS/1.2, HTTP/1.1`, not a redirect, because a redirect to a scheme the client did not ask for is how a downgrade goes unnoticed. Plain `ws://` survives **only** on a loopback bind for a single-machine rehearsal. |
| Where it terminates | **At the relay's front door** — the relay itself, or a fronting proxy that terminates for it. **That choice is operational and belongs to WP3**, not here. What this contract specifies is the wire-visible behaviour, and it is identical either way. |
| The certificate | Issued for the relay's DNS name by a CA the client's platform already trusts. A client MUST verify the chain, the name and the validity window, using its platform's trust store. |
| A certificate a client cannot verify | **Fail the connection and say why.** A client MUST NOT proceed, MUST NOT prompt, MUST NOT offer a flag that skips verification, and MUST NOT pin as a workaround. It logs one loud error naming the host, the presented name and the verification failure, and it retries on the ordinary backoff ladder — which will keep failing, which is correct: a certificate a client cannot verify is an operator problem at one end or the other, and there is no client-side action that makes it safe. |
| A rotation, seen from a connected peer | **Nothing.** A rotation replaces the certificate the *listener* presents to the **next** handshake; an established TLS session is unaffected and its WebSocket stays up. A peer sees a rotation only if the rotation restarts the process — and then it sees an ordinary disconnect, reconnects on the backoff ladder, and rejoins with `reason: "reclaimed"` (§7.2 rule 1). **The relay MUST be able to load a renewed certificate without dropping sessions**, and where it cannot, the rotation is a **routine restart** and D24's restart policy is what tells participants what it looks like (DQ2). |
| What TLS is not | Not authentication — B22 is. Not a reason to relax anything else: the frame checks, the `sourcePeer` comparison, `maxFrameBytes` and §3.3's limits all apply to a TLS connection exactly as they applied to a plain one. |
| The retired paths keep it | `/contract-b/v2` and `/contract-b/v3` are served over TLS like the live path (§3). A peer that cannot complete a handshake learns nothing, and the whole point of a retired path is that it teaches (`contract-a.md` §15, A23). |

**Why the operational half is deliberately left open.** A name, a certificate, an ACME client
and a renewal that the relay survives are WP3's work, and DQ2 lists them beside the supervisor,
the monitoring and the backup that a hosted service also needs and a desktop never did. This
amendment fixes only what a *client implementer* must do, because that is the part two
independent implementations have to agree on.

**Enforced by:** the **relay or its fronting proxy**, for the listener, the certificate and a
reload that does not drop sessions; **every client**, for verifying, for refusing to proceed
without verifying, and for having no flag that skips it; the **operator**, for the name, the
renewal and for stating in D24's announcement what a routine restart looks like.

### B24 — Capacity limits are a published table, and every one is a knob (§3.2, §3.3, §6.2, §6.3.1, §6.4, §6.5, §10, §10.1, §12)

*Contract Change 9 (D20's knob rule; DQ3).*

**Gap.** This wire had **no capacity limit of any kind** beyond `maxFrameBytes` and one
compiled-in genome rate. On a LAN of six known peers that was not a gap; on a public relay it
is the whole abuse surface. And the one limit of the right shape that did exist is the
instructive one: `contractb.GenomeRequestsPerMinute = 30` is a **compiled constant**, reachable
only by editing source — which D20 has already ruled on: *"a tunable an operator cannot retune
from the metric that measures it is not a tunable."*

**Resolution.** §3.3 is the table and is normative. Four rules govern it.

| Rule | Statement |
|---|---|
| **Countable at the frame level, or it is not a limit this relay may have** | Connections, frames per second, bytes per second, claims per minute, genome requests per minute, subscribers. **No limit may require the relay to decode a body**, index anything, or keep organism custody or domain state. B43 permits only bounded opaque transport state (amended — §28, B43). D1 is load-bearing. It is why the archive is a separate service and why M6 can replace the relay with libp2p. An abuse limit is not worth spending it (DQ3). |
| **Every one is a knob** | Flag and environment variable, no compiled constants. `genomeRequestsPerMinute` moves into the table as `maxGenomeRequestsPerMinute` and becomes one (§10, §12). |
| **Every peer-visible one is published** | The relay publishes the values **it is running with** — not the shipped defaults — in `HANDSHAKE_ACK.limits` at connect and `PEER_STATUS.limits` thereafter (§6.2, §6.5). A peer that does not know the ceiling it is measured against cannot be built to respect it, and a support conversation about a limit nobody can read is unwinnable. |
| **The relay sheds the connection, never the map** | An inbound-limit breach is close `4007` (§3.2). A claim-limit breach is `granted: false, reason: "rate_limited"` (§6.4). A full B43 migration queue is different (amended — §28, B43). It returns `NOT_FORWARDED` without closing the destination. A later connection loss can drop accepted queued migrations under §5.2's conservative forwarding records. `SLOT_VACANT` still means what §6.8 says. |

**Where the published table lives, and why it is not inside `stats`.** DQ3 asks for the limits
*"published on the stats block"*, on the model of D20 putting `inboundRatePerSimMinute` there.
**They are published beside it instead, on the same broadcast**, and the reason is that block's
own discipline: §6.3.1 is **peer-authored end to end**, the relay stores it as the bytes it
arrived as and interprets nothing (§16, B11), and a relay-authored key inside it would be the
first value on that block the peer did not write. The property D20 actually wants — **a number
is only readable against the cap it is measured on** — holds either way, because `limits` and
every peer's `stats` arrive in the same `PEER_STATUS` frame and the page renders them together
(§10.1).

A peer shed for capacity, in one frame:

```json
{
  "protocol": "contract-b/4.0",
  "type": "PEER_STATUS",
  "messageId": "e41a7b09-38cd-4f62-a5b7-0c9e1d43f820",
  "sentAt": 1785693845000,
  "data": {
    "epoch": 58,
    "map": { "width": 3, "height": 2 },
    "slotCount": 6,
    "slots": [
      { "slot": 2, "position": { "col": 1, "row": 0 }, "peerId": "peer-main-slot2",
        "live": false, "modConnected": false, "gameVersion": "0.6.3.1",
        "simulationSize": 2000.0, "exportEdges": ["E", "N"],
        "darkSinceMs": 1785693844880,
        "lastRefusal": "capacity: maxFramesPerSecond 50 exceeded (peak 412/s)" }
    ],
    "you": { "slot": 4, "position": { "col": 0, "row": 1 },
             "neighbours": { "E": 6, "N": 1 } },
    "observers": 1,
    "limits": { "maxFramesPerSecond": 50, "maxBytesPerSecond": 4194304,
                "maxClaimsPerMinute": 12, "maxConnectionsPerPeer": 2,
                "maxConnectionsPerAddress": 8, "maxGenomeRequestsPerMinute": 30,
                "maxSubscribers": 4, "maxFrameBytes": 8388608 }
  }
}
```

The close that produced it, on slot 2's own connection:

```
← close 4007 "maxFramesPerSecond 50 exceeded (peak 412/s over 3s)"
```

**Slot 2's neighbours lost a lane and nothing else.** Slot 1 re-targets east past it, slot 5's
column re-pairs, every in-flight migration addressed to slot 2 follows §9.2 unchanged, and the
map keeps running — which is the property that makes a capacity limit safe to have at all.

**Enforced by:** the **relay**, for counting at the frame level, for publishing what it runs
with, for `4007` and `"rate_limited"`, and for never letting a limit reach into a body; **every
client**, for reading `limits` at connect and being built to respect it; the **archive**, for
enforcing `maxGenomeRequestsPerMinute` on its own send path as it always has (§10); the
**operator**, for having a knob to turn at 03:00 instead of a rebuild.

### B25 — A minimum contract version at the handshake: a compatibility gate, never a security control (§3.2, §4, §6.1, §6.2, §6.5, §12)

*Contract Change 10, **additive half only** (D22).*

**Gap.** Compatibility on this wire is on the **major** alone, and that rule held for three
consecutive minors — the far end sat one behind and kept exchanging organisms. Then
`contract-b/3.3` broke the streak. `dev_environment.md`, *The minors*, records it: a pre-3.3
sidecar validates `MIGRATION_PAYLOAD.exitEdge` against `E`/`N` and answers `W` with a
**permanent** `MALFORMED_MESSAGE` NACK, so an upgraded neighbour's west and south exports were
refused, bounced and re-exported. **The two lanes into the stale peer ran at ~40 hops/min
against ~4–6 everywhere else**, two other slots pinned at `inboundQueueMax` and started
answering their own senders with `OVERLOADED`. Nothing was lost; the map was simply not
operationally complete until every peer upgraded.

The lesson `dev_environment.md` draws is the design input: *"a stale value range in a field
both sides already exchange breaks traffic; a stale absent optional field only ever costs a
number on a page."* **A public map cannot tell those two apart by itself, and the relay is the
only party that sees every peer's claimed version at once.**

**Resolution.**

| Rule | Statement |
|---|---|
| The gate | The relay MAY publish a `minContractVersion`, and MUST refuse a peer whose `protocolVersion` is below it: close `4003`, one log line naming both versions, and `lastRefusal` on that slot as `contract_version_below_minimum` (§6.1, §6.5). |
| It is published | In `HANDSHAKE_ACK` and on every `PEER_STATUS` (§6.2, §6.5), so a peer can read what refused it and an operator surface can say which peers are one release from being refused. |
| It defaults to nothing | Unset means **no minimum**. A floor is a deployment decision and a relay that has not made one must not enforce a guess. |
| It rises only after the release exists | D25 chose GitHub Releases, which **pushes nothing**, so there is no forced-update lever to design: **the fleet moves by publication** — a release, a matrix entry, and then a relay-side minimum. A floor raised before the release that satisfies it is a floor that ejects peers who cannot comply. |
| **It is a compatibility control and never a security control** | A version a peer *claims* is attacker-chosen text — §13 item 7 already forbids reading `contractAVersion` as a capability decision for exactly this reason. The gate keeps **honest** stale peers off a map they would degrade; it stops nobody who edits a string. **Both halves belong in the same paragraph** or an implementer will assume the first implies the second, and §6.1 states them side by side in a table for that reason. |
| It does not change §4's compatibility rule | Between **peers**, the minor is still never a rejection reason, unknown fields and types are still ignored, and a feature is still detected by the presence of its field. The floor is an **admission policy of one map**, not a statement about what two peers may assume of each other (§4). |
| It is not the support matrix | The matrix answers *which build runs on my game* and is settled on the operator's own machine; the gate answers *may this build join this map*. Two layers, two tests, and D22 is the decision that they never meet. |

**Why urgency rather than tidiness.** The peer that suffers from staleness **is not the peer
that is stale** — it is the neighbour whose exports bounce — and whether a fix has been applied
on somebody else's machine is unobservable from here (`dev_environment.md`, *The far end*). At
six peers you ask the operator. At sixty there is nobody to ask.

**Enforced by:** the **relay**, for the gate, the close, the `lastRefusal` and the publication;
**every client**, for reading the floor and for treating a refusal as an upgrade instruction
rather than a retry; the **operator**, for raising the floor only after the release exists, and
for never mistaking it for a security control.

### B26 — The relay acknowledges every forward, and the sender's own journal becomes the evidence (§5.2, §6, §6.12, §7.4, §9.2, §12, §13 item 6)

*Contract Change 6 (DQ2).*

**Gap.** §13 item 6 states it exactly: the forwarding record is in memory and does not survive
a relay restart, so **every outstanding `sent` entry loses its chance of a proof at that
moment** and falls back to the bounded 24-hour hold instead of re-routing in seconds. The item
names the fix — a receipt — and declines it: *"M4 does not buy that, because a relay restart is
rare and `--release-inflight` is one command."*

**Both halves of that argument are properties of a relay on the owner's desk.** On a hosted
relay (D24) restarts stop being rare — deploys, certificate rotation, kernel updates, the
supervisor doing its job — and `--release-inflight` is typed on the **sender's** machine, which
after M5 is usually a stranger's. The condition item 6 set was *"if this ever hurts"*, and the
change of venue is what makes it hurt, knowably in advance rather than after the first bad
night.

**Resolution.** §6.12 defines the message and §5.2 carries the rules; this is what they settle.

| Rule | Statement |
|---|---|
| One receipt per forward | Queue acceptance triggers one best-effort receipt to the **sender** (amended — §28, B43). The acceptance time also puts the `migrationId` in §5.2's record. A **re-route** produces another receipt. A retry from a sender older than §25's B37 does too. |
| It carries the session | `relaySessionId` rides the receipt, so the sender learns the **scope** of the fact with the fact (§5.2). |
| It lands in the journal | The sender records it durably against the entry (§7.4) and answers nothing. |
| **It changes no safety rule** | §9.2 is untouched in every particular. A receipt is **not** delivery, **not** custody, **not** proof of non-delivery, and it can never authorize a re-route. Its only direction is *toward holding*: an entry with a receipt was forwarded, so it holds. |
| **A missing receipt is silence** | Never sent, dropped from a full queue, or lost with the session — all indistinguishable. §9.2's *not evidence* table gains the row, because *silence is never proof in this contract* is the sentence this whole design rests on and a new frame is exactly the kind of thing that erodes it. |
| Not copied to subscribers | A receipt is a fact about one sender's journal, not about the migration. The fan-out set of §5.1 is unchanged. |
| Best effort | The relay MUST NOT delay, block or fail a forward on account of a receipt it could not send. |

**What it buys, in one sentence.** The sender no longer needs the relay to still be the same
process in order to know its own entry was forwarded — **and it still needs a live relay of the
right session to learn that an entry was *never* forwarded**, which is the direction a re-route
depends on and the direction a restart will always take away. That asymmetry is not a
shortcoming of the receipt; it is §5.2's *"a relay restart is exactly the event that
invalidates the proof"*, and a receipt that pretended otherwise would be a durable record
asserting what the new process cannot know.

**What it costs, and where that is measured.** One extra frame per migration, on a relay whose
whole design virtue is that it forwards frames and does nothing else (D1). At the rig's
measured 300–500 crossings a minute it is a rounding error. At a public map's rates it is a
real cost, and **WP3 measures it at rate rather than assuming it away** (DQ2) — which is also
why the frame is the cheapest one on this wire: four fields, no body, no fan-out, no answer.

**The relay enforces** one receipt attempt per accepted migration. A receipt never delays
a forward. **The sidecar enforces** durable receipt journaling and does not change a handoff
state for it. Both components leave §9.2's proof rules unchanged.

### B27 — The archive is an authorised subscriber, and the visibility boundary is stated (§5.1, §6.1, §6.3.1, §6.5, §10, §13 item 4)

*Contract Change 4 (DQ1).*

**Gap.** §13 item 4: *"The archive has no write interface and no authentication of its own. It
is a subscriber that trusts the shared token."* Under M4, `role: "archive"` was a
**self-declaration** — anyone holding the one token could open a socket, declare themselves a
subscriber and receive a byte-identical copy of every envelope on the map. And the question got
sharper twice after §13 was written: since §16 a copied `PEER_STATUS` carries **every world's
species census**, and since §19 it carries the **mod version, the save policy and the exclusion
list** (§6.3.1). **A public archive that copies every envelope to whoever asks is publishing a
fairly complete profile of a stranger's machine.**

**Resolution.**

| Rule | Statement |
|---|---|
| A grant, not a role | `role: "archive"` requires the **subscribe** grant on the credential (B22). Without it, `4003` and a reason naming the missing grant. **The grants are disjoint**: a subscribe credential cannot claim a slot and a peer credential cannot subscribe, so neither compromise becomes the other. |
| It is issued deliberately | At the relay's console, by the operator, like a join string. **No wire message asks for the grant and none confers it.** A public map has exactly as many subscribers as its operator decided to have, bounded by `maxSubscribers` (§3.3, B24). |
| **The boundary, stated rather than implied** | A subscriber sees every `MIGRATION_PAYLOAD`, `MIGRATION_ACK` and `MIGRATION_NACK` the relay routes or generates, and every `PEER_STATUS` — census, mod version, `contract-a` version, save policy, exclusion list, populations, depths and speeds, per world. That is the grant. It is written here so that **granting it is a decision** and so that D24's participant announcement can say what a participant is agreeing to before they join. |
| It is not a privileged view | A subscriber gets **nothing a peer does not already get**. Every field it reads is a field the relay already broadcasts to every sidecar on the map. There is no subscriber-only field, no private channel and no back door — which is what makes the boundary describable in one sentence. |
| The peer's own rule follows from it | **Nothing on this wire is confidential.** A sidecar that must not publish a value MUST NOT put it on the stats block (§6.3.1), because no rule downstream will hold it back. |
| Everything else about a subscriber is unchanged | Read-only, no answers, no sending, no claim, bounded queue, dedup on `migrationId` (and on `migrationId` + `code` for a NACK), and the relay closing a subscriber that stops reading (§5.1, §21 B21). B27 adds a gate at the door and changes nothing behind it. |

A client with a peer credential asking to subscribe:

```json
{
  "protocol": "contract-b/4.0",
  "type": "HANDSHAKE",
  "messageId": "c07f3d21-6b48-4a95-8e30-1f5b92d4a708",
  "sentAt": 1785693598200,
  "data": {
    "peerId": "peer-lan-slot5",
    "role": "archive",
    "protocolVersion": "contract-b/4.0",
    "gameVersion": "",
    "sidecarVersion": "0.4.0"
  }
}
```
```
← close 4003 "credential for peer-lan-slot5 does not carry the subscribe grant"
```

and nothing appears on slot 5's `lastRefusal`, because slot 5's *peer* connection was refused
nothing — this is a role error on one connection, not a refusal of that peer.

**Enforced by:** the **relay**, for the grant check at the handshake, for the disjointness, and
for `maxSubscribers`; the **archive**, for holding a subscribe credential and for going on
being read-only; the **operator**, for issuing the grant deliberately and for telling
participants what it lets its holder see (D24).

### B28 — An authenticated admin path for release, handover and eviction (§7.5, §13 item 5)

*Contract Change 5.*

**Gap.** §13 item 5: *"Release and handover are startup flags. If the map ever grows past what
one operator can restart at will, both need an authenticated admin path."* **The condition was
met by the venue rather than by the size.** A hosted relay's restart drops every peer's session
at once, so *at startup* is a price six innocent peers pay so that one slot can be released —
and after M5 those peers are strangers who were told what a restart looks like (D24) and are
now getting one for somebody else's problem.

**Resolution.** §7.5 carries the table; four properties make the path safe.

| Rule | Statement |
|---|---|
| **It is not on this wire** | A **separate listener**, never the Contract B WebSocket. No frame of §6's catalogue invokes an act, and no peer or subscriber can reach one. D1's relay routes frames and still does nothing else; the message catalogue grows by exactly one in this whole set, and it is B26's receipt. |
| Authentication is B22's, with a third grant | The **admin** grant, disjoint from peer and subscribe. Loopback by default, TLS if bound anywhere else (B23). |
| **The act stays deliberate** | Two calls. The first returns the **same consequence report** §7.5 has always required an operator to read — the slot, its position, its `peerId`, how long it has been dark, whose lanes change, which positions become holes — plus a single-use token bound to the act **and to the current `ring.json` state**. The second performs the act, and MUST be refused if the map moved underneath the token. **A confirmation an operator cannot see is not a confirmation.** |
| Every act is audited | One durable line: grant, act, slot, state before, and the operator's reason string. |

**A third act joins the two.** `--evict-peer <peerId> [--for <duration>]` closes a peer's
connection with `4005` and refuses it for a stated period. **It releases nothing** — the
reservation, the slot number and the position all survive — so an eviction is a **liveness**
act and the map treats an evicted peer exactly as it treats a dark one (§8). It is what DQ7
leaves for a peer that will not stop when suppressing its text at the renderer (B30) was not
enough, and it is deliberately the weaker of the two tools: suppression needs no peer's
cooperation and costs that peer nothing, and eviction takes a world off the map.

Release, over the path, in the two calls it takes:

```http
POST /admin/release-slot HTTP/1.1
Authorization: Bearer admin-ops.4b91c0e7d2a85f3168b0d47ac9e21356
Content-Type: application/json

{ "slot": 5, "reason": "peer departed 2026-08-09, operator request" }
```
```json
{
  "act": "release-slot",
  "slot": 5,
  "position": { "col": 1, "row": 1 },
  "peerId": "peer-lan-slot5",
  "darkForMs": 172800000,
  "becomesHole": [ { "col": 1, "row": 1 } ],
  "lanesChanged": [
    { "peerId": "peer-main-slot2", "edge": "N", "from": 5, "to": null },
    { "peerId": "peer-lan-slot4",  "edge": "E", "from": 6, "to": 6 }
  ],
  "addressRetiredForever": true,
  "heldEntriesAddressedHere": "not knowable from the relay — read stats.heldDepth on PEER_STATUS, and multiverse-sidecar --list-inflight --dest-slot 5 on the machine that holds the journal",
  "confirmToken": "3a0c7e91-52bd-4f16-9c48-7e10b8d3a624",
  "ringStateHash": "9f41c8a02e7b5d63"
}
```
```http
POST /admin/release-slot/confirm HTTP/1.1
Authorization: Bearer admin-ops.4b91c0e7d2a85f3168b0d47ac9e21356
Content-Type: application/json

{ "confirmToken": "3a0c7e91-52bd-4f16-9c48-7e10b8d3a624",
  "ringStateHash": "9f41c8a02e7b5d63" }
```
```json
{ "act": "release-slot", "slot": 5, "applied": true,
  "map": { "width": 3, "height": 2 }, "slotCount": 5,
  "maxSlotEverIssued": 7, "epoch": 62 }
```

**`heldEntriesAddressedHere` is a sentence and not a list on purpose.** The relay **cannot**
enumerate journals — they live on other people's machines and D2 keeps custody local — so the
field says what the relay does not know and where the answer is, which is the same division
§7.5 has always drawn. **After M5 the third row of that division changes hands**:
`--list-inflight` is typed on the machine that holds the journal, and that machine is usually a
stranger's, so the operator's honest answer becomes *ask the peer*.

**What this path deliberately does not become.** It is **not** the control surface. It changes
the map's **registry** — a reservation, an identity, a peer's admission — and touches nothing
inside a world: no time scale, no save policy, no exclusion list, no setting a mod reported.
D23 defers the surface that would write those to M6 and names the questions it must answer
(ordering against a world load, a disconnected target, authorization across machines D9 keeps
undriven, an audit trail); three acts on the relay's own registry answer none of them and claim
to answer none of them (§13 item 8).

**Enforced by:** the **relay**, for the separate listener, the admin grant, the two-call
confirmation bound to `ring.json`'s state, and the audit line; the **operator**, for reading
the report; **nothing on the peer wire**, which is the property that keeps D1 intact.

### B29 — Placement under churn: holes before growth, the shorter axis, and a broadcast bound (§1, §7.2, §7.5, §12, §13 item 3)

*Contract Change 8 (DQ6).*

**Gap.** §13 item 3: *"M4 places six known peers and splices one newcomer, by hand, once each.
Strangers joining and leaving continuously is M5's problem, and it is where auto-placement, the
coalescing window and `maxSlotEverIssued` growth will first be stressed."* The relay has
`--reserve-slot` and honours an advisory `insertAfterSlot`; **what it did not have is a rule
for a peer nobody expected**, and a public map is nothing but those.

**Resolution.**

| Rule | Statement |
|---|---|
| **Holes before growth, on both claim routes** | Rule 6 always filled a hole before extending an axis. **Rule 4 now does too**: a `preferredPosition` that would extend an axis while any hole exists is ignored and falls through to rule 6. A preference is an operator's layout on the rig and an ordinary stranger's configuration file on a public map, and one newcomer that extends an axis creates `height` (or `width`) positions and fills exactly one of them. |
| It does not break the rig's layout | The join kit's six positions each extend only when the rectangle is full, so every claim in that sequence is granted exactly as before (§7.2). That is the test of the narrowing rather than a footnote to it. |
| **Which axis extends** | The **shorter** one, unchanged, and the reason is now written down: the cycle length on each axis is what genetic mixing depends on, so a map stretched along one axis is a map whose other axis has nothing to route around (§2.1). |
| A departed peer's position and address part company | The reservation never expires and a returning peer lands where it was; the position becomes fillable **only** through `--release-slot`, and then only as an ordinary hole. **The address is retired forever** — `maxSlotEverIssued` never decreases — because `SLOT_VACANT` is a permanent answer and therefore a valid proof of non-delivery (§6.8), and reissuing an address would silently convert that proof into a lie. This is exactly the split D12 and D13 built (DQ3). |
| **The broadcast rate is bounded, twice** | The coalescing window **widens** from `statusCoalesceMs` toward `statusCoalesceMaxMs` while a window sees more than `statusChurnBurstThreshold` changes, and narrows again after a quiet one; the last frame of a burst is still always sent. And **a repeat claim that changes nothing structural broadcasts nothing** — the claimant still gets its `SECTOR_GRANT`, and everybody else is not told. |
| The epoch rate had a measured cause | The living deployment's slot 6 issued **64** placement claims in one day against two or three from each local slot, every one a re-claim with `reason: "updated"` as its measured time scale wandered (DQ3). Not a reconnect and not a defect — **64 epochs on a six-slot map, from one peer, for nothing**, and the second rule above is what stops that scaling with peer count. |
| **A consequence, stated rather than discovered** | An organism routed around a bypassed slot never sees that world, so a map with continuous churn has a **continuously shifting cycle**. D12 chose route-around deliberately, so this is not a defect — but the genetic mixing a public map produces is **not** the mixing a stable map of the same size produces, and whoever reads the archive later should know which one they are looking at. |

**What this amendment does not do is test itself.** `maxSlotEverIssued` growth has **never**
been run at any scale, and neither the widened window nor the suppressed re-claim has been
measured under churn. **WP5 runs a synthetic churn harness — join, leave, return, never return
— to exhaustion before WP8 involves anybody**, because Risk 2's accepted cost is that the exit
test cannot be re-run cheaply: it spends other people's goodwill each time.

**Enforced by:** the **relay**, for the narrowed rule 4, the widening window, the suppressed
re-claim broadcast and the monotone `maxSlotEverIssued`; the **sidecar**, for sending a
`preferredPosition` it is content to lose (§7.2 — a claim is advisory in every part and never
fails for a lost race); the **operator**, for `--release-slot` being the only way a position
comes back.

### B30 — Attacker-chosen text: the escaping obligation is testable, the version prohibition sits where a page implementer reads it, and moderation stops at the view (§10.1, §13 item 7)

*Contract Change 7 (DQ7).*

**Gap.** §13 item 7 names the surface and is blunt about the split: the wire's answer is the
shape check and the cap, and it is done; *"the renderer's answer is its own, and a page that
interpolates a name into HTML without escaping it has a defect this contract cannot fix for
it."* Two problems with leaving it there. **An escaping rule written in a contract is not
code** — nothing in this project asserts it. And the rule lives in §13, a section titled *Open
items for M5*, which is not where anybody implementing a page is reading.

**Resolution.** §10.1 gains three rows — the section that states what the page may claim — and
this is what they settle.

| Rule | Statement |
|---|---|
| The inventory, complete | `species[].genericName` and `.specificName` (64 UTF-8 bytes each, 32 per peer), `migrationExclude[]`, `modVersion`, `contractAVersion`, `lastRefusal`, `lastSave.name`, **and the two the contract never counted because they predate the concern — `peerId`, and the world name a player chose.** |
| The obligation | Escape for the surface rendered into — HTML, an HTML attribute, a URL, JSON in a script, terminal escape sequences — and never render one as markup. `ringstat`'s terminal is a rendering surface with its own injection story and is bound identically. |
| **The test, which is the point** | A peer reports a species named with markup; the rendered page and `ringstat`'s output are asserted against it, **in CI and not by eye**. That test is WP7's, and it is the only form in which this rule is true of a running system. |
| The version prohibition, repeated where it is read | A reader **MUST NOT** parse `contractAVersion`, `modVersion` or a peer's `gameVersion` into a capability or refusal. A peer that can choose the string can choose the capability it claims. **Detect a feature by the presence of its field** (`contract-a.md` §3.1). It is the same rule B31 puts on the envelope's game version, and the four exceptions B31 keeps are gates in the relay, the walk and the mods — **never in a page**. |
| **Moderation: the view, not the record** | The page and `ringstat` MAY apply an operator-side **deny list at render**, suppressing a string without evicting the world that produced it — no peer cooperation, no wire field, no contract change — and B28's eviction stays available for a peer that will not stop. **What cannot be promised is removal from the record**: D11 and §10 make the ledger a thing nothing evicts from. **M5 promises removal from the view and explicitly does not promise removal from the record.** Anything stronger is a change to D11's never-evict rule and belongs in a decision row, not in a support reply. |

**Why the deny list is the right minimum.** A species name is a string a player typed or the
game generated, published to every subscriber and rendered on a page anyone with the URL can
read, and until M5 there was no operator action between *ignore it* and *evict the peer*.
Suppression at render costs the suppressed world nothing, needs nothing of the wire, and leaves
the record intact — which is the only combination that does not contradict something this
system is built on.

**Enforced by:** the **page and `ringstat`**, for escaping every listed field and for the deny
list; **CI**, for the markup-species-name test, which is what makes the first row a fact rather
than an intention; the **archive**, for going on recording verbatim (§10) — the record is not
where suppression happens; the **operator**, for the deny list's contents and for not promising
a takedown the design forbids.

### B31 — The envelope's game version is diagnostic metadata, D22's layering is stated, and four shipped gates are kept as named exceptions (§4, §6.6, §6.10, §8, §9.2, §10, §13 item 7)

*Contract Change 10a (D22, refined and closed 2026-08-11).*

**The layered statement, first, because everything else here follows from it.** D22 is the
owner's own design, in his words: *"game version compatibility should work basically what
matters is like the sidecar version compatible with the relay server right then it depends if
theres a sidecar version compatible to the game version."* Read as a design it is **two tests
that never meet**:

- **The relay's test — the wire is the membership test.** The relay cares that each sidecar
  speaks a compatible **contract** version and about nothing else. That is the only question
  the map has an opinion on, and B25 is the mechanism.
- **The machine's test — a support matrix, not a fleet pin.** Each operator needs a sidecar and
  mod build compatible with the game version *they* run. The project publishes a matrix over
  game versions, and the test is settled on the operator's own machine before anything dials
  the relay.

**This supersedes the fleet-wide same-game-version rule** as a *design*. The old rule was
unenforceable and misplaced at once: Steam auto-updates stay on and land unevenly, so *every
peer runs the same build* stops being true within hours of an update and there is nobody to
tell — while a map-wide pin would eject honest players for something they did not choose and
could not defer.

**Gap.** The envelope has carried the serializing game's version since M2, end to end and as a
REQUIRED field — `contract-a.md` §5.3's `MIGRATE_OUT.gameVersion`, this document's
`body.version`, `parents[].gameVersion`, `MIGRATE_IN.gameVersion`, the sidecar's journal and
the archive's ledger, whose live lines carry `"gameVersion":"0.6.3.1"` today. **Nothing had to
be added to carry it.** What was missing was a statement of what it is *for*, and the owner
supplied it on 2026-08-11: *"for now lets just keep doing the payload opaque and assume it will
work, we will worry about doing the normalized own schema or the cheaper alternative in the
future if we run into an issue, for now we can just assume this will work and carry the game
version info for potential future debugging."*

**Resolution.**

| Rule | Statement |
|---|---|
| The payload stays opaque | D4 unchanged and unextended. **Cross-version loading is assumed to work.** No refusal path is designed on the game-version axis, no gate is added, and the stability research is not done now. |
| The field is diagnostic | `body.version` and `parents[].gameVersion` are carried so a future incident is **diagnosable from the record** instead of reconstructed from memory. **A reader MUST NOT parse either into a capability or refusal decision** — §13 item 7's `contractAVersion` rule, applied to the second version axis. |
| **It binds NEW readers only** | This is the narrowing that makes the rule honest. Four shipped mechanisms already decide on the game version; the owner chose the same day to keep every one of them. So the prohibition binds **readers written from `contract-b/4.0` onward**, and the four are stated below as **kept, deliberate exceptions** rather than as violations nobody intends to fix. |
| Both fuller answers are deferred, not rejected | This project's own **normalized canonical schema**, and the cheaper **marker-plus-refusal**. Neither is built until an incident makes one necessary. |
| The archive decides nothing from it | It records `body.version` against every migration and MUST NOT refuse a record, split a lineage, filter a fetch, order a queue or mark a peer on the strength of it (§10). |
| `genome-hash.md` is untouched | The projection takes the version tag as an **identity** input, not a capability one — two game versions produce two hashes for one organism, deliberately, because gene names and `NodeType` ordinals are version-scoped (`genome-hash.md` §10 item 2). An identity computation is not a refusal decision and this rule does not reach it. |

**The four kept gates, named, with where each fires:**

| Gate | Where it fires | What it does |
|---|---|---|
| §9.2's `VERSION_UNSUPPORTED` | The **importing mod**, on a payload whose `body.version` has no `bb8-schema` dialect | Answers `MIGRATE_IN_NACK` / `VERSION_UNSUPPORTED`, **permanent**; §9.2 makes it normative — *do not re-deliver, hold for an operator, mark the peer pair incompatible*. **This is the marker-plus-refusal design, already shipped.** |
| `mapwalk`'s `peer_incompatible` skip | The **routing walk**, in the sidecar and in the relay's own neighbour walk (§8) | Skips a peer on a different game version, which §8 aggregates into a **closed edge** and `contract-a.md`'s `EDGE_STATUS` reports to the mod. |
| Close `4002` | The **mod ↔ sidecar handshake** (`contract-a.md` **§2.1's close table**; **that document's rule, named here and not authored here** — `contract-a.md` §21, A48 keeps it as a named exception) | Refuses a mod whose `CONFIG_UPDATE.gameVersion` has no dialect. Inert in practice — the allow-list is empty outside tests and empty means accept — and normative. |
| §6.1's relay refusal | The **relay**, at connect and at claim | Closes a mismatched handshake with `4003`, answers a mismatched `SECTOR_CLAIM` with `version_incompatible`, and skips a mismatched peer in its own neighbour walk. **§22 does not touch it** (§6.1). |

**What a version-skewed map looks like, written here because it is the accepted behaviour and
an operator will meet it.** Two machines on different game builds **do not exchange organisms**.
The relay refuses whichever peer disagrees with the version the map already reports, at connect
or at claim; `mapwalk` and the relay's own walk close the edges between mismatched pairs; and
the importing mod answers a permanent `VERSION_UNSUPPORTED` if a payload ever reaches it. **That
is safe** — no cross-version payload reaches the loader at all — and it is exactly why the
assumption above stays untested rather than validated. The everyday shape of it is a **partition
along a version boundary after a staggered game update**, and it ends when every machine is on
the new build. The rig has always lived this: re-syncing both computers promptly after a game
update is what ends the partition (`dev_environment.md`, *The far end*).

**The cost of keeping the gates, stated rather than discovered.** *Assume it works* and *keep
the gate that stops it being tried* compose into an assumption **that can never be exercised**:
a gate that refuses every cross-version crossing means the ledger can never record one. So the
signal that reopens this design space is **not** a failed restore — it is the **partition
itself**: refused claims, `lastRefusal` on a slot, `peer_incompatible` edges and
`VERSION_UNSUPPORTED` NACKs, counted after a staggered game update. **WP8 watches for that
instead of for a crossing** (DQ5).

**And one contradiction is held open knowingly rather than resolved.** D22 says the map holds no
opinion about the game version; §6.1 says the relay MUST refuse on it, and the relay enforces
that in code. **Both statements stand.** The owner's call of 2026-08-11 was to leave the shipped
gates alone and reconsider when skew causes a real operational problem, so this document records
the disagreement in the open — in §6.1, in §8, in §9.2 and here — rather than papering it over
with wording that no running component obeys. The whole cross-version design space reopens
together, or not at all.

**Enforced by:** **every new reader**, for treating the field as a note; the **archive**, for
recording it and deciding nothing from it; the **four gates**, for going on doing exactly what
they do; the **operator and WP8**, for counting the partition rather than waiting for a crossing
that the gates guarantee will never happen.

### B32 — `contract-b/4.0` is a major bump, the path moves to `/contract-b/v4`, and the fleet crosses in lockstep (§3, §4, §6.1, §6.2, §12, header)

*Contract Change 15 (D21).*

**Change.** §3.1's rule is explicit about what costs a major: additive fields raise the minor;
**a changed rule with an installed base** does not. B22 removes the shared token and replaces it
with a per-peer credential, and §3.1's own framing and `contract-a.md` §15's A41 both put that
on the expensive side.

**Resolution.** Apply the contract's own test honestly, item by item:

| M5 change to Contract B | Kind | Needs a major? |
|---|---|---|
| §3.1's shared token replaced by a per-peer credential bound to the `peerId` (B22) | **a rule replaced, with an installed base** | **yes** |
| §3.2's `4006` narrowed to require the credential (B22) | an existing code's precondition changes | **yes** |
| Transport becomes TLS; `ws://` refused off loopback (B23) | the transport a client must speak changes | **yes** |
| `FORWARD_RECEIPT` added (B26) | additive message type — a receiver that does not know it ignores it (§4) | no |
| §3.3's limit table; `limits` on `HANDSHAKE_ACK` and `PEER_STATUS` (B24) | new section, additive OPTIONAL and REQUIRED fields on relay-authored frames | no |
| Close `4007`; `SECTOR_GRANT.reason` gains `"rate_limited"` (B24) | additive close code, additive enum value | no |
| `minContractVersion` published and enforced (B25) | additive field; an **admission policy**, not a compatibility rule (§4) | no |
| Subscriber grant (B27), admin path (B28) | an authorisation rule and an off-wire surface | no |
| Rule 4 narrowed to holes-before-growth; the widening window (B29) | placement and broadcast behaviour, no wire shape | no |
| §10.1's escaping, version and moderation rows (B30) | page-input rules, off the wire | no |
| `body.version` restated as diagnostic (B31) | a prohibition on new readers; no field, no type, no enum | no |
| Message catalogue otherwise, envelope, custody, dedup, the hold, the fan-out, hashing, routing inputs | **all unchanged** | no |

Three rows are enough. **The identifier is `contract-b/4.0`**, and by §3.1's rule the URL path
moves with the major to **`/contract-b/v4`**.

**The retired paths, and the same rule for the same reason as `contract-a.md` §15, A23.** A
relay MUST keep serving `/contract-b/v2` and `/contract-b/v3` and MUST close every connection
on one immediately with `4000 PROTOCOL_UNSUPPORTED`. A bare HTTP 404 is a socket error in a log
and half an evening of diagnosis; `4000` is a defined close the client already handles by
logging one loud error and not reconnecting (§3.2). **The retired paths are served over TLS**
(B23) — a peer that cannot complete a handshake learns nothing, and the point of a retired path
is that it teaches. There is no field-level fallback anywhere: a `contract-b/3` peer cannot be
made to work by presence-detecting a credential field, because the *rule* changed and not the
shape, and a compatibility path only an already-rejected peer can take is dead code that reads
like a supported configuration.

**The document's own identity moves with it, and its filename does not.** The title stops
naming a milestone, because a document titled *Contract B — M4* that specifies M5's wire is
false on its first line; `contracts/contract-b-m4.md` **stays the path**, because every document
in this project cites it by name. The header's box applies the same three tests that split
`contract-b-m3.md` off — milestone-scoped? structural? is the old text still needed as it
stands? — and answers *in this file*, which is the answer `contract-a.md` §15's box gave for
`contract-a/2.0`, for the same reason: **the version identifies the wire, the file identifies
the interface.**

**Contract A takes a minor in the same wave.** A bearer token is an **additive precondition on
an existing handshake**, so by that document's own test it is `contract-a/2.4` — no path move,
no rebuild of the mod's message catalogue. Its matching set is `contract-a.md` **§21, A47–A52**,
authored in the same wave, and it carries Contract Changes **3, 11, 12, 13 and 15** plus
Contract A's half of **10a** in A48. **The rollout is where the two sets meet in practice:**
A47 is a **mod** change, so the rig's crossing to `contract-a/2.4` takes all five local games
down at once — which is the same window this document's crossing needs, and the reason both
belong in one deploy rather than two (`contract-a.md` §21, A52, *The migration note for the
living deployment*).

#### The migration note for the living deployment

**The fleet that crosses this major is six worlds on two machines, and it moves in lockstep.**
That is D21's whole reason for taking the major **now**: this is the last moment a breaking wire
change costs a rebuild instead of a migration story for people the owner cannot reach (D13,
applied one milestone later). **It is also the only rehearsal this project gets** for moving a
fleet across a major before the peers are strangers, and it should be run as a rehearsal rather
than as a deploy.

**What a fleet does to cross a major, in order:**

1. **The sidecars and the relay move together.** A `contract-b/3` sidecar and a `contract-b/4`
   relay are incompatible **by design**; there is no rolling window in which the map is half
   crossed and still exchanging organisms. Every peer needs its credential **before** the relay
   stops accepting the shared token, which means the join strings are minted and distributed
   first, while the old wire still works.
2. **The old path stops answering, loudly.** `/contract-b/v3` goes to `4000` at the moment the
   relay restarts on `contract-b/4.0`. A peer left behind gets a defined close and a log line,
   never a silence — which is the difference between an upgrade somebody notices and one they
   diagnose.
3. **The far end applies it at its own operator's leisure, and that is not observable from
   here.** D9 forbids the rig to drive the second computer. The one observable is the
   `client gone` / `client connected` / `reason=reclaimed` triple for that peer in `relay.log`,
   and until it appears, that slot is dark and its lanes are bypassed — working exactly as
   designed, and looking exactly like a peer that has gone away (§8, Risk 5).
4. **The far end gets a bundle built from the changed source**, because a stale far-end sidecar
   is what refuses an upgraded neighbour's exports — `dev_environment.md`'s *The minors* is the
   episode, and B25's gate is the rule written from it.

**The rollout shape this specific rig has, stated because it changes the cost.** Contract A's
bearer token ships in the same milestone and is a **mod** change, and a mod deploy takes all
five local games down at once — `deploy.sh` copies into `BepInEx/plugins/` and a running game
locks the DLL. **So the rig's crossing to `contract-b/4.0` is a five-games-down deploy, not a
rolling sidecar change**, and every other pending mod change belongs in the same window.
`m5_tracking.md` carries the measured cost and the sequence; the contract's part is only to say
that the crossing is one window rather than six.

**What the crossing must not cost, and it is the bar WP2 reports against: zero discarded
journal bytes.** Every sidecar comes back to **its own slot and its own coordinate** with
`reason: "reclaimed"`, replays its journal with **zero** discarded bytes, and reopens all of its
lanes — because a credential change touches the wire and not custody, and a peer that loses a
journal entry to an authentication upgrade has lost an organism to bookkeeping (D2).

**Enforced by:** both wire ends, symmetrically — each side sends `"contract-b/4.0"` and compares
only the major; the **relay**, additionally, for the retired paths and their `4000`; the
**operator**, for minting the credentials before the old wire stops, for one deploy window
rather than six, and for reading the far end's return out of `relay.log` rather than assuming
it.

## 23. The archive's retention horizon (2026-08-12)

**Decision 3 was answered by the owner on 2026-08-12**, and it is the one M5 decision this
document had already written a place for. §10's closing paragraph — *And one limit that is now
on somebody else's timetable* (added — §22, B27) — states the constraint the answer had to live
inside: **nothing here may evict**, and *"a retention rule that contradicts that is a change to
D11 rather than a configuration of it."* The answer contradicts it on exactly one of the two
halves, so this is that change, recorded and bounded:

> **The migration ledger is kept forever. The genome BLOBS are pruned to a horizon**, default
> 30 days on the hosted deployment, a knob everywhere.

**The narrowness is the whole design.** D11's rule about the **record** is untouched — every
crossing, every hash, every lineage node, every species block and every `GENOME` line naming
who served what is still permanent, and nothing in this set gives any component a way to remove
one. What gains a horizon is the **bytes**: the content-addressed store under
`<data-dir>/genomes/`, whose entries are a cache of blobs the ledger already describes. The
archive's lineage graph keeps all of its nodes and ages out some of its leaves' contents, which
is the shape `m5_considerations.md`'s Decision 3 paragraph and `m5_tracking.md`'s row both
record.

**Two amendments, B33 and B34**, continuing the `B` series for the reason §14 gives. They are
not separable and §23 is one set for that reason: an eviction pass without B34's rule re-fetches
what it has just deleted, forever, and B34's rule without a horizon has no number to measure.

**This set does not change the wire.** No message type, no field, no enum value, no NACK code,
no close code, no change to custody, dedup, the hold, the fan-out, hashing, routing or admission
control — and no change to what any peer sends or accepts. **§4's test therefore answers neither
major nor minor, and the identifier stays at `contract-b/4.0`.** The wire consequence is stated
rather than added, and §6.10 had already defined it: a holder that does not hold a hash answers
`found: false, reason: "unknown_hash"`, which that section calls **"a normal answer, not an
error"** and whose listed causes already include *"the source may have restarted and lost its
cache"*. A pruned hash is that case, reached deliberately. And on this deployment it is not even
reachable from the wire: the archive is a **read-only subscriber** (§5.1, §22 B27) that answers
no `GENOME_REQUEST` at all, so its pruning is observable only in its own gap report and on its
own status page. A peer running with a horizon and a peer without one are indistinguishable to
every other party on the map, which is the same test §20 and §21 applied to their own sets.

**What it costs, stated once, out loud.** Risk 7 stops being a hazard and becomes a **policy**:
*a genome nobody fetched inside the horizon is permanently unfetchable.* D11's *known limit*
framing already prepares a reader for a record that is incomplete by construction, and this
makes one class of that incompleteness deliberate and dated instead of accidental. §22's B30
line is unchanged and is worth reading beside this one: **M5 promises removal from the view and
does not promise removal from the record** — a horizon prunes blobs by age and is not a takedown
mechanism, has no way to name a peer, a species or a hash, and must never be offered as one.

### B33 — The genome store gains an operator-set retention horizon, off by default; the ledger's rule is untouched (§10, §12, §20 B20, §22 B27)

*`m5_considerations.md` Decision 3, answered 2026-08-12. Milestone-internal — no D-row of its
own — and it moves D6: the catalog inherits a complete graph whose leaves age out.*

**Gap.** §10 said the archive's store "is bounded by neither tunable above — it is the record,
and nothing evicts from it", §20's B20 named it in *What stays unbounded is named*, and §22's
B27 put the sizing problem on the hoster's timetable. All three were written when the archive
was one person's, on one volume, for six worlds. A public archive's genome store grows with peer
count **and** with crossing rate, the operator who hosts it has signed for a bounded run (D24),
and the 2026-08-08 ENOSPC outage is what running out looks like. The decision the owner faced
was: keep everything and buy the disk, or keep the record and let the bytes age.

**Resolution.**

| Rule | Statement |
|---|---|
| **The ledger never evicts, at any setting** | ~~`migrations.jsonl` keeps every record forever, and no configuration of this horizon may touch one.~~ **Superseded on its scope, kept on its point — §26, B39/B40.** The original text is left above rather than rewritten, because it is what this set decided on 2026-08-12 and a reader of §23 must see it. What holds unchanged is that **no configuration of the GENOME horizon may touch a line**; what §26 adds is the ledger's *own* window (`ledgerWindow`, §12), a separate knob under a separate set. The rest of this row is untouched **for the life of the line, and no longer**: **the record of what the archive held outlives what it holds** — "we had it, `peer-lan-slot4` served it, it aged out" — stays answerable from the `GENOME` line for the window and from the off-host copy for the run, and it is **not** on B39's normative list of folds kept forever. **A per-hash provenance question is a window question**, and §26 says so out loud rather than letting a reader infer otherwise from this row. D11 is unchanged and §20 B20's rule that a replay must read past permanent damage still applies, segment by segment. |
| **The genome store gains a horizon, and it is OFF at the contract level** | `genomeRetentionHorizon` (§12) is **unset by default**, and unset means M4's behaviour exactly: nothing evicts, no pass runs, no counter appears. A **deployment** turns it on — the M5 hosted run sets `720h` — which keeps the default on the side where a mistake costs disk rather than data. An implementation MUST refuse a negative value and MUST treat an unparsable one as unset, for the same reason. |
| **The horizon is measured from last stored or last served** | Not from the crossing, and not from the file's creation. The store already refreshes an entry's mtime on a re-store and on every read (§10's least-recently-served cache rule), so the horizon runs from the last time anybody wanted the blob. **A blob served inside the horizon keeps its whole horizon** — it is never evicted mid-horizon — and a store that measured from first write would delete a genome served an hour ago. |
| **A pass is bounded in work, and it yields** | §21's B21 discipline, applied to a sweep instead of a fetch, and mandatory for the same reason. One pass MUST examine a bounded number of shards and entries, remove a bounded number, resume where the last pass stopped over a **stable** order, and release the store's own lock at least every chunk of removals. The archive's fetch pump calls into that store **while holding the lock its relay read loop needs**, so a sweep that held the store's lock across a whole pass would starve the read loop through the far end of the same chain — and a subscriber that stops reading is one the relay closes. That incident cost 7,789 crossings. |
| **A removal happens under the store's own lock, with the age re-read inside it** | Reading a directory is not deciding: an entry listed as old may have been served before the delete lands. The deletion MUST take the lock the store's own reads and writes take, and MUST re-read the entry's age inside it, or the "never evicted mid-horizon" rule above holds only for blobs nobody wanted. |
| **The scratch files go on the same walk** | §20's B20 requires the store's expiry sweep to collect abandoned `<hash>.json.tmp` files, and the archive's store **never had a sweep to do it with**. The pass collects them as it passes, under the same age rule that made `staleTmpAge` safe. |
| **A pass says what it removed** | Count and bytes, in the log, and as counters on §10.1's status surface beside `genomeGaps`. An operator MUST be able to tell **"a horizon is set and nothing has aged out yet"** from **"this archive prunes nothing"**, so the horizon itself is published: absent means off, which is §10.1's unknown-is-a-value rule applied to a knob. |
| **The wire is untouched, and §6.10 is why** | A pruned hash answers exactly like a hash the holder never had: `found: false, reason: "unknown_hash"`, already a normal answer with a lost cache already among its causes. No field, no type, no enum value, no code — see this section's version note. |

**Enforced by:** the **archive**, for the bounded, resumable, lock-yielding pass, for the
mtime axis, for keeping the ledger out of it entirely and for publishing what it removed; the
**genome store**, for taking its own lock around a removal and re-reading the age inside it;
and the **operator**, for choosing the horizon deliberately and for knowing that the number they
choose is the age at which this system stops being able to answer "what did that organism look
like".

### B34 — A gap older than the horizon stops being retried, and the retirement is counted (§10, §21 B21, Risk 7)

*`m5_considerations.md` Risk 7's first mitigation, taken — "bound the ladder and record the
abandonment as a fact the operator can count, instead of retrying forever".*

**Gap.** §10's fetch table says *"Keep the hash forever"* and *"a fetch that failed for a year
can succeed tomorrow"*, and its retry ladder therefore has no bottom. Risk 7 is what that costs
when a peer leaves for good: a throughput-limited backlog becomes a permanent one, and §21's
incident is what a permanent backlog does to a subscriber at the ×100 crossing rate. **Under
B33 the retry becomes worse than futile**: a blob won past the horizon is one the very next
eviction pass deletes. The queue finally has a drain, and it only exists if both mechanisms read
the same number.

**Resolution.**

| Rule | Statement |
|---|---|
| **One horizon, both mechanisms** | The gap queue and the store MUST use the **same** horizon. Two numbers, or a horizon on one side only, produce an archive that re-fetches what it just evicted — indefinitely, at the fetch rate, for every departed peer's backlog. **Three mechanisms since 2026-08-17** (amended — §26, B40): the ledger's window joins them and reads the same number, because a gap is only ever queued for a crossing inside the horizon, so a window equal to the horizon holds exactly the crossings whose gaps can still be fetched. |
| **The gap's clock is the crossing's recorded time** | Not the moment this process first noticed the gap. A requester's "first seen" is set by its own replay and therefore **resets at every restart**, so a retention rule built on it would make every gap younger than the last reboot and would never retire one. The archive already records `recordedAt` on every migration; that is the crossing's own time and it survives everything. |
| **A retired gap leaves the queue and stays in the record** | Retirement removes the entry from the **retry set** and from nothing else. §10's *keep the hash forever* is a rule about the ledger and is untouched: the hash is still a lineage node, still in the gap report, still `[MISSING]` in the read path. **What stops is the asking.** |
| **A replay MUST NOT re-queue a crossing already past the horizon** | The startup replay hands every hash in the ledger to the tracker, so without this rule an archive rebuilds its whole retired backlog on every restart and pays for it in resident memory as well as in work — which is the term `wp3_hosting_options.md` measures against the box's RAM. |
| **Retirement is a fact the operator can count** | `genomeGapsExpired` on §10.1's status surface, in the style of `genomeGaps` beside it, plus a summary in the log. Risk 7's accepted cost is a lineage graph with holes for departed peers; a counter is the difference between a stated policy and a silence. |
| **Off by default, again** | With no horizon set, nothing is retired and the ladder of §10 runs exactly as it always has, forever. B21's bounds are untouched in either case: retirement happens **inside** the same bounded, yielding, round-robin walk and adds no unbounded work to a pass. |

**Enforced by:** the **archive**, as the only requester on this wire, for measuring a gap on the
crossing's own clock, for retiring inside the bounded walk rather than in a pass of its own, and
for counting what it abandoned; and the **operator**, for reading `genomeGapsExpired` as the
number that says how much of the map's genetic record has passed out of reach.

---

## 24. The wire's own bytes (2026-08-16)

**This set is a measurement, not a decision that was waiting for an owner.** On 2026-08-16 a
45-second capture was taken between the front-door proxy and the relay on the hosted deployment,
and it answered a question nothing in §1–§23 had asked: *what does this map cost to run, in
bytes?*

> Seven peer connections to `/contract-b/v4` consume **~2,508 GiB a month** — 1,364 relay→peer
> and 1,144 peer→relay — against a host allowance of **3,072 GiB**. That is **82%** of the
> budget, spent by eight sockets.

**Where it goes is the shape this document already describes**, which is what makes the two
amendments below narrow:

| Type | Share | Mean frame | Who pays for it |
|---|---|---|---|
| `MIGRATION_PAYLOAD` (§6.6) | 82.7% | ~17.9 KB — `body.bb8` 15.4 KB, of which the brain is 15.1 KB | one sender, one destination, and the archive's copy (§5.1) |
| `PEER_STATUS` (§6.5) | 11.6% | 12.8 KB | **every peer and every subscriber, per broadcast** |
| `GENOME_RESPONSE` (§6.10) | 3.3% | 17.7 KB | one requester |
| everything else | 2.4% | small | — |

**Nothing was compressed at any layer, and the capture proves it out loud**: JSON keys were
readable in the packets. TLS does not compress; the front door sets `gzip off` because a
WebSocket is not a response body; and §3 forbade the one mechanism the transport has. Offline
deflate over the same capture — zlib level 6, 32 KiB window, per-stream context takeover, which
is the analogue of what RFC 7692 negotiates — returned **9.0x** on `MIGRATION_PAYLOAD`, **9.8x**
on `PEER_STATUS`, **10.4x** on `GENOME_RESPONSE` and **8.5x overall**, which is ~2,508 GiB
becoming ~295 GiB. Without context takeover it is ~4.5x.

**Two amendments, B35 and B36, and they are independent.** B35 makes the transport compress;
B36 fixes a relay that has been sending `PEER_STATUS` at four times the contract's rate. Either
one lands without the other. They are one set because the same capture found both and because a
reader who applies only one will misread the remaining number.

**This set does not change the wire.** No message type, no field, no enum value, no NACK code,
no close code, no change to custody, dedup, the hold, the fan-out, hashing, routing or admission
control — and nothing any peer sends or accepts changes shape. **§4's test therefore answers
neither major nor minor, and the identifier stays at `contract-b/4.0`**, exactly as §23's did.
The reasoning is worth stating because "a rule is being replaced" is §22's own test for a
**major**, and this replaces a MUST NOT:

1. **§4's identifier is a statement about FRAMES.** It tells two peers what they may assume of
   each other's envelopes and fields. A negotiated transport extension is settled on the HTTP
   upgrade, before the first frame exists, and it is invisible in every frame afterwards — the
   receiver hands its application identical bytes either way.
2. **The replaced rule has an installed base that does not notice.** §3.1's shared token was the
   expensive shape because a `contract-b/3` peer **stops working** against a `contract-b/4`
   relay. Here every deployed peer keeps working unchanged, byte for byte, because it never
   offered the extension and the relay is required to serve it anyway. A rule whose replacement
   cannot break an existing peer is not the shape §22 B32 priced.
3. **A bump would do active harm.** B25's `minContractVersion` lets an operator refuse peers
   below a published floor. Moving the identifier for this would let a floor refuse a peer for
   lacking something it already interoperates without, and would tell a reader a frame changed
   when none did. **RFC 7692's own handshake is the feature detection**, it works in both
   directions between two `contract-b/4.0` peers, and a second detection mechanism on top of it
   would be a thing to get out of step.

**Contract A takes no set** (`contract-a.md` §2, unchanged, and its `permessage-deflate` MUST
NOT stands). That wire is loopback between the mod and the sidecar on one machine: its bytes
cross no network, appear in no transfer bill, and buying a per-connection megabyte of deflate
state in every participant's game host would be a cost with no matching saving. **Contract C
takes no set either**: it is the `MigrationEnvelope` carried *inside* `data` (§6.6), it names no
transport, and nothing here reaches it.

### B35 — `permessage-deflate` MAY be negotiated, the relay offers it, and an old client is served uncompressed forever (§3, §12)

*From the 2026-08-16 transfer measurement. Milestone-internal — no D-row of its own — and it
moves D24: the bounded hosted run's dominant cost stops being the wire.*

**Gap.** §3's transport table has said `permessage-deflate` **MUST NOT** be negotiated since
`contract-b/1`, and **no document in this project has ever given a reason for it**. The line was
written for Contract A's §2 — a loopback wire, with the honest qualifier *"Not used in M2"* — and
was copied into `contract-b-m2.md` the same evening without the qualifier, then into `-m3` and
`-m4` unchanged. It was inherited, not reasoned, and it was inherited from the one wire where it
is correct. Meanwhile this wire became a public map over TLS on a metered host carrying 17.9 KB
of JSON per organism, and §3's own rows moved with it — plain HTTP became TLS, a LAN port became
`443`, a shared token became a per-peer credential — while the compression row did not move
because nobody had a number.

**Resolution.**

| Rule | Statement |
|---|---|
| **The extension is permitted, and it is RFC 7692's, unmodified** | `permessage-deflate` **MAY** be negotiated on the HTTP upgrade. This document defines no framing, no parameters and no fallback of its own: the extension's own handshake is the whole mechanism, and anything this contract invented on top of it would be a second thing to keep in step. |
| **The relay OFFERS it and ACCEPTS it** | Both halves. §3's table gives the relay the server role, so it is the party that echoes `Sec-WebSocket-Extensions`, and the saving is on both directions — 1,364 GiB out and 1,144 GiB in. |
| **A client that does not offer it MUST be served, uncompressed, and MUST NOT be refused — ever** | This is the load-bearing rule of the whole set and it has no expiry. This project moves its fleet by publication and cannot make a participant upgrade (D25, `release/README.md`); a relay that required the extension would evict every world that had not. There is no minimum version to raise for this and B25's floor MUST NOT be used as one. |
| **Both ends must offer it or it is not used** | Which is RFC 7692's rule and is restated because it is what makes the compatibility rule above cost nothing to keep: an old sidecar and a new relay produce **exactly** the pre-§24 wire, on the same URL, with the same frames, and no code path exists that is reachable only in that combination. |
| **Context takeover is the default, on both halves** | It is the difference between ~8.5x and ~4.5x on this traffic, and this wire is the case the mode exists for: long-lived connections carrying near-identical JSON, where the second `PEER_STATUS` costs a back-reference rather than a dictionary. |
| **The cost is per connection and it is named** | ~1.25 MB of resident deflate state per connection per process — a sliding window plus a held compressor. At the hosted map's eight sockets that is ~10 MB against a 512 MB ceiling. **At hundreds of peers it is not**, and the fallback is `client_no_context_takeover` / `server_no_context_takeover` — roughly half the ratio at near-zero fixed cost — rather than turning the extension off. An implementation MUST NOT be written so that the mode is stated in more than one place. |
| **The relay has an off switch; a client does not** | `wireCompression` (§12), `--ws-compression` / `MULTIVERSE_RELAY_WS_COMPRESSION`, on by default. It is complete on its own, because the extension needs both ends: a relay that stops offering it returns the whole map to uncompressed frames as peers reconnect, with no participant action and no rollback. A client-side knob is **forbidden** for B23's reason — no metric in a participant's hands answers to it — and would in any case only be able to make one world's traffic worse. |
| **Nothing above the transport may read it** | No frame says whether it was compressed, no `PEER_STATUS` field reports it, and no routing, custody, dedup, hashing or admission decision may consult it. A compressed peer and an uncompressed peer are indistinguishable to every other party on the map, which is the same test §20, §21 and §23 applied to their own sets. |
| **§3.3's limits keep measuring DECOMPRESSED bytes, and that is the safe half** | `maxFrameBytes` and `maxBytesPerSecond` (§3.3, §12) MUST be enforced on the frame the application receives, **after** inflation. Measuring the compressed size would let one peer spend a 4 MiB/s allowance from a few kilobytes of network and hand the relay a fan-out it never authorised: a `PEER_STATUS` costs `slotCount` blocks to everybody whatever the sender paid to send its stats. It also closes the obvious abuse — a frame that inflates past `maxFrameBytes` is refused at the ceiling it was always refused at, and closes 1009 as §3.2 says, so the extension adds no way to make a relay allocate more than a frame. **The consequence is stated rather than hidden**: these two limits stop being a bound on this peer's share of the transfer bill and are only ever a bound on the relay's work. They were sized as the latter (§22, B24) and this changes nothing about them. |
| **A refusal at the door negotiates like an acceptance** | The upgrades a relay completes only in order to close them — a retired path (§3), a capacity shed (§3.3) and an eviction (§22 B28) — MUST answer the extension exactly as the live path does. Compression saves them nothing, since a close frame is a control frame and RFC 7692 never compresses one; they share the answer so that `Sec-WebSocket-Extensions` cannot separate "refused before the handshake" from "accepted, then closed". B28 requires an evicted peer to see what a draining relay shows it, and a distinguishable handshake would be a refusal shape B28 does not grant. |

**Enforced by:** the **relay**, for offering the extension at every door and for never refusing a
client that declines it; **every client** — the sidecar and the archive — for offering it and for
holding no knob that turns it off; and the **operator**, for owning `wireCompression`, for the
memory headroom the mode's per-connection state needs, and for knowing that a relay updated
alone saves nothing until the clients that dial it offer the extension too.

### B36 — A stats arrival schedules no broadcast; `statsBroadcastIntervalMs` is the only publisher stats reach (§6.5, §7.2, §12, §14 B4, §22 B29)

*From the same capture. It is a **defect**, not a design change: the implementation diverged from
this document, and the document is unchanged in intent.*

**Gap.** §6.5 says `PEER_STATUS` is sent after every registry change and "also sent on a
`statsBroadcastIntervalMs` timer, because stats change without the registry changing", and §14's
B4 fixes that timer at 5 000 ms and explains that it exists precisely so the stats do not need
another trigger. §22's B29 then bounds the broadcast **rate** and defines the thing it counts as
a **registry** change, listing seven and excluding a stats block by name.

§7.2's own rule 2 then said it for the `SECTOR_CLAIM` path in as many words — *"its stats ride
the next `statsBroadcastIntervalMs` timer, which was going to send them anyway"* — and said
nothing about the `PING` path, which is where §6.11 actually puts the block.

**Every one of those four places assumed what none of them said**: that a stats arrival is not
itself a reason to publish. The shipped relay bumped its pending-broadcast counter on every
stats-bearing `PING`, and stats arrivals do not align — `statsIntervalMs` is per peer, so seven
peers produce seven arrivals per interval in seven different coalescing windows. The measured
result was **1.32 `PEER_STATUS` broadcasts a second** where §6.5 designs for one per five
seconds: **~4x the intended rate**, ~11.6% of the map's whole transfer bill, ~246 GiB a month, and
a number that grows with the peer count in **both** terms — more arrivals, each costing more
blocks. The relay's own source said the rule out loud beside the code that broke it: the stats
timer is "a floor on freshness, not a second source of frames".

**Resolution.**

| Rule | Statement |
|---|---|
| **A stats arrival is stored and publishes nothing** | A stats block on a `PING` (§6.11) or on a `SECTOR_CLAIM` (§6.3) is recorded against its peer with its `statsAsOfMs`, and that is the whole of the work. It **MUST NOT** raise the epoch, **MUST NOT** schedule a broadcast, and **MUST NOT** count toward B29's churn — the last was already true and is restated so the three cannot drift apart again. §7.2 gains it as a third rule, beside the two B29 wrote, because that is the section an implementer builds the publisher from. |
| **The timer is the only publisher stats reach** | `statsBroadcastIntervalMs` (§12), and it is unchanged: 5 000 ms, matching `statsIntervalMs`, a compiled default with no flag. §14 B4's interaction note now reads exactly: coalescing bounds how closely two broadcasts may follow each other, this timer bounds how far apart they may drift, and **between them there is no third source**. |
| **A registry change still publishes immediately, coalesced** | Unchanged, and it is what keeps the fix narrow: a placement, a release, a handover, a peer arriving or going dark, an eviction and a refusal written to a slot are still published inside `statusCoalesceMs`, and the last frame of a burst is still always sent (§7.2, §22 B29). The map does not become slower to report what it IS; it stops re-reporting what it SAYS. |
| **Stats are deferred, never dropped** | The block is on the peer's record from the moment it arrives, so the next broadcast — timer-driven or registry-driven — carries the latest one. A reader that sees a registry change sees fresh stats with it. |
| **What freshness costs, stated** | Stats on the map age from ~0.76 s to at most `statsBroadcastIntervalMs`, 5 s. §12's `statsStaleMs` is **30 s**, so no reader's staleness rule changes and nothing that rendered as state renders as unknown. `statsAsOfMs` is untouched in meaning — §6.5 defines it as the relay clock when the block **arrived**, never when it was published — so a reader ages the stats from the arrival either way, and §10.1's unknown-is-a-value rule is unaffected. |
| **The saving is stated so the two amendments are not double-counted** | ~246 GiB a month on the 2026-08-16 figures, taken **before** B35 compresses what is left. A reader applying both must apply B36 first and then B35's ratio to the remainder, or `PEER_STATUS` is credited twice. |

**Enforced by:** the **relay**, which is the only party that publishes `PEER_STATUS` and the only
one that can get this wrong; and the **archive and every operator surface**, for continuing to age
a stats block by `statsAsOfMs` rather than by the frame it arrived in — which is what makes a 5 s
publication cadence indistinguishable from a 0.76 s one to every reader that follows §6.5.

---

## 25. Migration is dangerous (`contract-b/4.1`, 2026-08-17)

**This set removes a mechanism rather than adding one, and the owner asked for it in one
sentence.** On 2026-08-17 at about 00:15Z:

> *"yeah just remove the hold in the reforwarding"* — and, on the reasoning:
> *"we don't mind much on bibites being lost, we can see migrating as dangerous, and then there
> wouldn't be duplicates while making things simpler."*

**What that reverses.** §9 has carried, since the owner's sign-off of 2026-08-05, exactly one
bounded exception to at-most-once: an organism forwarded to a slot that then went dark, with no
proof of non-delivery, was **held** for `holdTimeoutMs` — 24 hours by default — and then
**bounced home by itself**. The exception was accepted knowingly and its residual duplication
case was named in the contract (§9.3). Everything the map grew around it — the `held` handoff
state, the accrual in the journal entry, the retry that fed the proof (§14, B5), the
exponential backoff that bounded the retry (§14, B8), the automatic bounce, `heldDepth`,
`bouncedTimeoutTotal`, `duplicateSuspected` and two §12 tunables — existed to keep that one
exception bounded.

**The same owner removed the exception on 2026-08-17, and the mechanism goes with it.** The
trade is stated in the direction the owner stated it: **losing an organism is acceptable, and
duplicating one is not.** Migrating is the dangerous act, and it is dangerous in one direction
only. That is a smaller system, not a differently-tuned one: the branch that could produce two
organisms is gone, so nothing has to bound it, count it, report it or explain it.

**Two amendments, and they are independent.** **B37** removes the hold. **B38** bounds the
archive's duplicate set, which was unbounded *because* a re-forward could arrive a year later
and B37 is what stops that. Either lands without the other, and a deployment that takes B37
alone is correct and simply keeps paying for a set it no longer needs.

**The version answer is in §4**, argued line by line the way §22's B32 and §24 each argued
theirs: no frame changes, both directions of a mixed fleet interoperate, one additive OPTIONAL
stats field is added and two are retired rather than removed. **The identifier is
`contract-b/4.1` and `/contract-b/v4` does not move.**

**Contract A takes no set and no version change.** A bounce-back is still an ordinary
`MIGRATE_IN` with `bounceBack: true` on the origin's own exit edge (`contract-a.md` §5.7,
§15 A25), the mod has no timeout concept and needs none (`contract-a.md` §16), and the
`MIGRATE_IN_NACK` hold at the DESTINATION — an arrival refused permanently and parked for an
operator (`contract-a.md` §13, A7) — is a different mechanism that shares a word and is
untouched. What Contract A does take is **marked text**: its §13 A6 row 3 and its D2 mapping
row now point here.

### B37 — The bounded hold, the re-forward and the timeout bounce are removed; at-most-once carries no exception (§1, §4, §5.1, §5.2, §6.3.1, §6.6, §6.7, §6.8, §6.12, §7.4, §7.5, §9, §11, §12, §13 item 6, §14 B5, §14 B8)

*Owner decision, 2026-08-17. It reverses the M4 sign-off of 2026-08-05 recorded in D2 and in
§9, and `system_decomposition.md` D2 carries the reversal on its own side.*

**Gap.** None. This is not a defect being fixed or a conflict being reconciled; it is a design
decision being taken back. The mechanism worked, was tested and was correct against the rule it
was built for. The rule changed.

**Resolution.**

| Rule | Statement |
|---|---|
| **A committed attempt is enqueued once** | Once a sidecar durably commits `sent` for one `(destSlot, rerouteCount)` attempt, it MUST NOT enqueue that attempt again. A matched proof can select a new alternate. Every cadence, handler, custody tick, and restart uses the current durable state (amended — §31, B46). |
| **Silence resolves to a loss, never to an action** | An entry in `sent` with no terminal answer within `forwardTimeoutMs` becomes a **`lost` tombstone**: the organism is gone, `stats.lostForwardTotal` increments, and one error-level line names it (§9.3). Nothing is delivered to any mod. |
| **The `held` state is removed** | A dark destination is no longer a state change for a forwarded entry. A journal written before this set replays a `held` entry as **`sent`**, which is what it always meant; an implementation that replayed it as an unknown string would leave the entry in no case of its own state machine. |
| **Nothing accrues** | `holdTimeoutMs` and `holdAccrualFlushMs` are removed from §12, and `accruedHoldMs` from the journal entry. `sentAt` is a plain wall-clock instant for the current or most recent committed attempt. It is durable and written before socket enqueue. A proven refusal or reroute clears it before a pending alternate is recorded (amended — §31, B46). A wall clock is safe **because expiry no longer moves an organism** — it closes a record, so a sender that slept through the deadline records a loss that had already happened. |
| **Proof-based re-routing STAYS, in full** | A `pending` entry re-routes on the sender's own record. A `refused` entry re-routes under a current peer NACK or matched relay proof. **A statement is not silence.** B46 requires exact queue-attempt correlation and a same-attempt receipt check (§31). |
| **And it has an off switch, for an owner who wants one** | `maxReroutes` (§12) takes a **NEGATIVE** value meaning *never re-route*: an entry that would have re-routed bounces home instead. It is a knob rather than a code path precisely so this choice can be revisited without a release. |
| **The remaining bounces are all proof-based** | §9.4 loses the ambiguous timeout. It keeps a payload refusal, an entry that reached nobody and has no lane, and the operator's `--release-inflight`. B46 adds the bounded exact queue-refusal exhaustion path. **No automatic bounce can duplicate an organism.** |
| **`--release-inflight` becomes the only duplication path** | It stays, unchanged in shape. Its printed risk text **MUST** say that it is the only way left to duplicate an organism on this map, rather than one of two ways (§9.3). |
| **A late `MIGRATION_ACK` is good news and MUST still be read** | Against a `lost` tombstone it means the organism arrived and its answer outran the deadline. Exactly one organism exists. The sidecar logs it, counts it and names the remedy — raise `forwardTimeoutMs` — and `duplicateSuspected` is removed, because the case it counted cannot occur. |
| **The stats block: one field added, two retired** | `lostForwardTotal` is added (§6.3.1). `heldDepth` and `bouncedTimeoutTotal` are **retired**: a `contract-b/4.1` sidecar stops sending them, a reader renders them unknown, and **their names are reserved and MUST NOT be reused**. |
| **The destination and the archive change nothing** | Dedup on `migrationId` against the journal and its tombstones (§6.6), the narrow re-ACK rule (§14, B6) and the archive's own dedup (§5.1) all stand exactly as written. They were defences against a duplicate arriving; duplicates still arrive, from peers older than this set and from defective ones, and these are what make them harmless. **A conforming implementation MUST keep every one of them.** |

**What it costs, named rather than buried.** Three cases that used to end with the organism
alive now end with it lost:

1. The destination took custody, died before its acknowledgement, and never came back.
2. The destination took custody, died, and came back **before** the old 24-hour timeout — under
   the old rule the sender's retry would have found it. There is no retry.
3. A `sent` entry left by a sender that **crashed between its durable send commit and the relay's answer**.
   §14's B5 is the full argument: the retry was the proof's only conduit for an entry a
   previous process wrote, so `sent → refused → re-route` is unreachable for that entry. The
   window is milliseconds wide and the consequence is one organism.

**What it buys.** The system has no duplication case at all. Every branch that could produce a
second organism is deleted rather than bounded, so nothing has to keep the bound correct: no
clock with two conditions, no accrual to persist and compact, no retry cadence to tune, no
backoff to cap, no `duplicateSuspected` metric, no error-level line explaining that the map may
now hold two copies of something. **The map's entity-ID count becomes an invariant instead of
an exit-test measurement.**

#### The transition, and why it needs no lockstep

**Old sidecars keep working, and they keep holding and retrying.** This project moves its fleet
by publication and cannot make a participant upgrade (D25, `release/README.md`). A sidecar built
before this set, against a `contract-b/4.1` relay and archive:

| It does | The map's answer |
|---|---|
| Re-forwards a frame every `forwardRetryMs` | The relay applies B43 like any other attempt (amended — §28, B43). Accepted copies enter the destination's paced queue and trigger a best-effort `FORWARD_RECEIPT`. A full queue returns `NOT_FORWARDED`. The destination still deduplicates any copy that arrives. |
| Sends a duplicate to a destination that already journaled it | The destination deduplicates and answers **nothing** (§6.6, §14 B6). Unchanged. |
| Sends a duplicate to a destination that already delivered it | The destination re-ACKs with `duplicate: true`. Unchanged. |
| Sends a duplicate copy to the archive through the fan-out | The archive refuses it inside `archiveDedupWindowMs` (§25, B38). |
| Holds an entry for 24 hours and bounces it home | Its own world gets its own organism back, and may duplicate it exactly as §9.3 said before this set. **Nobody else's world is affected.** The exception was always local to the sender that ran it. |
| Publishes `heldDepth` and `bouncedTimeoutTotal` | Ignored. The names are reserved, so nothing else claims them. |

**Nothing breaks in the other direction either.** A `contract-b/4.1` sidecar against an older
relay never uses the retry that relay expected; the relay forwards, receipts and NACKs as
before, and its 48-hour forwarding record is simply consulted less often.

**The release order is therefore free, and one order is still better.** Relay and archive first
— they are the operator's own machines and they carry the dedup that absorbs old senders — then
the sidecar release, at each participant's own pace. **`minContractVersion` (§22, B25) MUST NOT
be raised to `contract-b/4.1`** to force the crossing: it would evict a world for running last
month's build in order to apply a change that costs that world nothing.

**Enforced by:** the **sending sidecar**, which is the only party that can get this wrong and
the only one that changes; the **receiving sidecar and the archive**, for keeping every dedup
rule exactly as it is, forever, because the peers that need them are the ones this map cannot
upgrade; and the **operator**, for `forwardTimeoutMs`, for `maxReroutes`, and for reading
`lostForwardTotal` as a map fact rather than as a fault.

### B38 — The archive's duplicate set becomes a bounded window (§5.1, §10, §12)

*From B37. It is the one structure whose unboundedness had a reason, and B37 removed the
reason.*

**Gap.** §5.1's duplicate set holds one key for every record the ledger carries, and
**nothing was ever deleted from it**. That was correct and deliberate: `migrations.jsonl` is
never rewritten, so a re-forwarded migration recorded a second time would put a permanent
duplicate row in the record, and a re-forward could arrive at any distance in time. The set was
therefore the one retained structure that grew with the ledger — 128 MiB at the 2026-08-16
production size of 5.4 M keys, measured, and the largest single term in the archive's resident
memory (`deploy/SIZING.md`).

**B37 removed the re-forward.** A conforming sender hands each migration to the relay exactly
once. What can still produce a second copy of one record is a **sidecar older than B37** running
its retry loop, and a **defective peer** — and both of those arrive within seconds or minutes of
the original, never a year later.

**Resolution.**

| Rule | Statement |
|---|---|
| The window | `archiveDedupWindowMs` (§12), default **172 800 000 ms (48 hours)**. A duplicate that arrives inside it is refused, exactly as before. |
| It is a FLOOR, not a deadline | The set is two generations rotated on the window, so a key is remembered for **at least** one window and **at most** two. A correctness argument may rely on the floor and nothing else. |
| What it costs to exceed it | A duplicate that arrives later is **appended to the ledger a second time**, and the ledger is never rewritten. That is a duplicate ROW in the permanent record and never a duplicated organism, and it is stated as the price rather than hidden as a defect. |
| Memory | At most **two windows of keys**, whatever the age of the record. At the 2026-08-16 production rate the arithmetic is in `deploy/SIZING.md`, and the model is a **constant** rather than a function of the ledger. |
| Nothing is deleted from a generation | Which is what preserves the open-addressed table's whole reason for existing: no tombstones, no degradation, 25 bytes a key. The oldest generation is **dropped whole** and its memory returns in one piece. |
| The seeds are per WINDOW, not per generation | Two generations that hashed a key differently would answer different questions, and a rotation would silently forget what it was meant to keep. |
| The replay rebuilds the window, never the record | A restart inserts only the keys whose records fall inside the window, walking the **ledger's own timestamps**. The cost of a restart stops growing with the age of the record, and the answer is the same one the live path would have given. |
| The ledger is untouched | B38 bounds a **memory structure**, and that is still exactly what it does. ~~`migrations.jsonl` is still kept forever~~ — **amended — §26, B40**: the lines gain a window of their own under a separate set, the aggregate they fold into is kept forever (§26, B39), and D11 is not in question either way (§10, §23 B33). B38's own window is unchanged by that and stays `archiveDedupWindowMs`. |

**The operator's knob, and when to move it.** Raise `archiveDedupWindowMs` while the fleet
still holds sidecars older than B37 **and** the archive has the memory for it; the cost is
linear in the window. Lower it on a memory-bound archive once the fleet has crossed. **Zero is
not a value**: an archive with no window would record a duplicate row for every retry a single
old peer sent.

**Enforced by:** the **archive**, and nobody else. No frame changes, no peer can observe the
window, and no other component holds a copy of this set.

---

## 26. The record's two halves (2026-08-17)

§23 recorded the owner's answer to Decision 3 and was careful to say what it did not touch:

> **The migration ledger is kept forever. The genome BLOBS are pruned to a horizon.**

That narrowness was right for the question it answered, and it left one thing unexamined —
**whether "the ledger" and "the record" are the same object.** They are not, and the difference
is what this set is.

The ledger is a file of per-crossing lines. The record is what those lines are folded into: who
crossed, how often, from where, descended from what, first seen when. The archive already
computes the second from the first at every start and **has never written it down**, so the only
way it can answer any question is to keep every line forever. **This set writes the answers
down.**

> **The record — the aggregates, the ancestry and the statistics — is kept forever. The
> per-crossing lines are kept for a window equal to the genome horizon, compressed once closed,
> and removed only after an off-host copy is confirmed.**

**The owner ratified it on 2026-08-17**, in the shape it is written here, against a measurement:
at the deployment's own rate the per-crossing lines are `1.0`–`1.3 GB` a day, and *"kept for the
whole run and beyond"* was **not funded to the end of its own announced run on the host it was
made on**. A bigger volume moves the date and changes nothing about the shape. What this set
does instead is stop paying for the answer twice — once in the lines, and once in the fold the
archive rebuilds from them at every start.

**What is given up is stated once, out loud.** A question nobody was asking while a segment was
on the host **cannot be asked of that period afterwards**, except of the off-host copy. That is
a real loss, it is not reduced by careful implementation, and it is why **B40 makes the off-host
copy a precondition of removal** rather than a recommendation beside it. What is **not** given
up: no crossing disappears from the counts, no species loses its history, no ancestry edge is
dropped, and the ancestry floor keeps meaning what it meant.

**This set is not a takedown mechanism and must never be offered as one.** §22's B30 line stands
exactly as written: **M5 promises removal from the view and does not promise removal from the
record.** A window ages lines **by date**. It cannot name a peer, a species, a hash or an
organism, and a request to remove one is answered exactly as it was before this set existed.

**Two amendments, and they are one set.** **B39** makes the aggregates durable. **B40** gives
the lines a window. They are not separable for the reason §23 gave about its own pair: a window
without a durable fold destroys the answers along with the lines, and a durable fold without a
window solves nothing the deployment is actually failing at.

**B41 was drafted and is withdrawn.** It would have bounded the archive's duplicate set, which
was unbounded only because a re-forward could arrive a year later. **§25's B38 reached the
contract first and did it**, sized against how far behind the map's oldest peer may be —
`archiveDedupWindowMs`, 48 hours — rather than against this set's shorter draft. Restating it
here would put two numbers in the contract for one structure. **The number B41 is spent and MUST
NOT be reused**, on the same rule §25 applied to `heldDepth` and `bouncedTimeoutTotal`.

**This set does not change the wire.** No message type, no field, no enum value, no NACK code,
no close code, no change to custody, dedup, the fan-out, hashing, routing or admission control.
The archive is a **read-only subscriber** (§5.1, §22 B27) that answers no `GENOME_REQUEST` at
all, so a windowed archive and an unwindowed one are indistinguishable to every other party on
the map; the window is observable only on the archive's own status page. **§4's test therefore
answers neither major nor minor, and the identifier stays at `contract-b/4.1`** — §25 moved it
and this set does not, on the precedent §23 and §24 each set for themselves.

### B39 — The aggregates become durable and are kept forever (§5.1, §10, §10.1, §20 B20, §23 B33)

*Owner ratification, 2026-08-17. Milestone-internal — no D-row of its own — and it moves D6 and
D11 on the same side §23 moved them: the catalog inherits a complete fold whose raw lines age
out.*

**Gap.** The archive computes the whole record at every start and writes none of it down. Every
number on the status page, every species row, every ancestry edge and every evidence item is a
fold over `migrations.jsonl` held in memory, rebuilt by replaying the file from its first byte.
That is why the file could never be shortened: **the only durable copy of the answer was the
question.** It is also why an archive restart is a participant outage that grows with the record
— the relay is held down for the whole replay — and why the replay's cost at the announced end
of the run is measured in tens of minutes rather than in the `90 s` it costs today.

**Resolution.**

| Rule | Statement |
|---|---|
| **The record is the aggregate, and it is durable** | An implementation that windows its ledger **MUST** persist every fold that its surfaces and evidence read. It **MUST** keep these folds forever. The species folds include crossing counts, sighting times, distinct-genome counts, lineage instances, and the earliest parent record. The other folds include lane counts, peer counts, record totals, unreadable-line totals, brain coverage, eviction counters, and gap counters. This list is normative. If a fold is absent before segment retirement, the archive cannot compute that answer for the period. The lineage-instance meaning is amended by §29, B44. |
| **A missing or unreadable roll-up is a loss and says so** | It **MUST NOT** be treated as a run of zeroes. The archive rebuilds what it can from the segments it still holds, publishes the earliest point its knowledge now begins at, and says out loud what it lost — the same shape the brain sidecar's loss rule and the genealogy's ancestry floor already use (§10.1, *unknown is a value*). **A zero would be a claim about the world made out of a failure of the record.** |
| **The roll-up is written incrementally** | A full rewrite on a timer costs the size of the whole state every time it runs. The file **MUST** append what changed and **MUST** be rewritten only when it exceeds a fixed multiple of its live content. This is the rule the brain sidecar already states, and it is here for the same reason: one persistence discipline in the package, not two. |
| **The aggregates never contradict the segments they still hold** | Loading the roll-up and replaying the records behind its cut **MUST** produce the same state as replaying every record it covers. This is a testable property, it is the one a validation phase exists to prove, and it is the only defence against a fold site that stopped marking itself changed. |
| **The ancestry floor stays fixed, because it is an aggregate** | The earliest record that ever named a parent is on the list above, so the floor does **NOT** become a rolling date when the window turns over. A surface that reports it **MUST** report the earliest crossing this archive ever recorded with a parent, not the earliest one it still holds a line for. |

**Enforced by:** the **archive**, for marking every fold write site as changed, for the
append-and-compact discipline, for the loss rule and for publishing where its knowledge begins;
and the **operator**, for reading a roll-up loss as a hole in the record rather than as a quiet
day.

### B40 — The ledger gains a window, and removal is gated on an off-host copy (§10, §12, §20 B20, §23 B33, §23 B34)

*Owner ratification, 2026-08-17: **"B+D, 30-day raw window"**, with the off-host copy in the
project's own cloud account. It is the change §10's closing paragraph named in advance —
a retention rule for the ledger is a change to D11 rather than a configuration of it, so this
is that change, recorded and bounded.*

**Gap.** `migrations.jsonl` grows with traffic and nothing shortens it. §20's B20 named it among
the unbounded files and put the sizing problem on the operator; §22's B27 put it on somebody
else's timetable; §23 left it alone deliberately. The measurement of 2026-08-16 is what closes
the question: the remaining announced run is more new ledger than the service host's volume
holds, so the promise was going to be broken by a full disk rather than by a decision.

**Resolution.**

| Rule | Statement |
|---|---|
| **One horizon, now three mechanisms** | The genome store's eviction, the fetch queue's retirement and the ledger's window **MUST** read the **same** number. §23's B34 gave the reason for the first two; the third joins them because a gap is only ever queued for a crossing inside the horizon, so a window equal to the horizon holds **exactly** the crossings whose gaps can still be fetched, and the fetch queue rebuilds from the window with nothing lost. **A shorter window abandons live gaps silently. A longer one buys nothing.** |
| **The window is off by default** | Unset means the ledger is kept whole, which is the behaviour of every release before this set, and it keeps the default on the side where a mistake costs disk rather than data. A **deployment** turns it on — the M5 hosted run sets `720h`, which is `genomeRetentionHorizon`'s own value by the rule above. An implementation **MUST** refuse a negative value and **MUST** treat an unparsable one as unset. |
| **A segment is closed, then compressed, then never rewritten** | Rotation **MUST** be atomic within the directory and **MUST NOT** rewrite a record. At every point of a rotation, and after any crash during one, **at least one complete copy of every record MUST exist**. A startup **MUST** reconcile a rotation it finds half-done rather than assume one. |
| **A segment is NOT removed until its off-host copy is confirmed** | The window is a bound on the host, not a decision to destroy. Removal **MUST** be conditional on a recorded confirmation that the segment exists somewhere the host's own failure does not reach. An implementation with no such confirmation **MUST** keep the segment and **MUST** publish that it is over its window. **This is the rule that makes the window a capacity measure rather than a deletion.** |
| **The window is published** | The horizon in force, the oldest record still held, the record count inside the window and the count of segments awaiting an off-host copy **MUST** appear on the status surface, in the style of `genomeGaps` and `genomesEvicted` beside them. An operator **MUST** be able to tell *"a window is set and nothing has aged out yet"* from *"this archive windows nothing"* — §10.1's unknown-is-a-value rule applied to one more knob. |
| **A replay reads past damage exactly as before** | §20's B20 is unchanged in effect and its reasoning is restated rather than assumed: a damaged line is skipped and every record behind it is kept. **A segment is never rewritten**, so damage inside one is as permanent as damage in the single file was, for the life of that segment. What changes is that the damage leaves the host with its segment instead of being read past forever, and that B39's count of unreadable lines outlives both. |
| **The window is not a takedown, and cannot be made into one** | It selects **by date and by nothing else**. An implementation **MUST NOT** offer, and an operator **MUST NOT** accept, a request to retire a segment early, or to exclude a peer, a species, a hash or an organism from one. §22's B30 is the rule and this row is only its restatement at a new surface. |

**What it costs, named rather than buried.** Three things become true and stay true:

1. **Per-crossing questions become window questions.** *"Which individual crossed when"* is
   answerable for the window on the host, for the run in the off-host copy, and on no live
   surface at all.
2. **A new aggregate cannot be added retroactively** to a period whose segments have gone. Cheap
   while an off-host copy exists; impossible if it does not. This is the strongest argument for
   making that copy a policy rather than a courtesy, and it is why B39's list is normative.
3. **The evidence claim moves with it.** *"Exactly once, proved by an entity-identifier census
   over the whole ledger"* stops being computable on the host once the window turns over. Since
   §25's B37 the claim is **at-most-once by construction, with losses counted** — no component
   re-forwards, no component bounces, a migration to a dark destination is a **counted loss**,
   and the evidence is `lostForwardTotal`, `duplicatesRefused` and the per-lane totals B39 keeps
   forever. **A missed record is not reconstructible from anything**, and no surface may imply
   that it is.

**What it buys.** Disk stops being a date and becomes a flat number. An archive restart stops
being a participant outage that grows with the record and becomes a few seconds, permanently,
because B39's fold is loaded rather than recomputed. And the off-host backup of the record stops
being a project and becomes a daily upload of one immutable segment.

**Enforced by:** the **archive**, for the atomic rotation, for the crash reconciliation, for
gating removal on a recorded confirmation and for publishing the window; the **operator**, for
choosing the number deliberately, for keeping the off-host destination real, and for knowing
that the number they choose is the age at which this system stops being able to answer *"which
organism was that"*; and **every participant-facing document**, for stating the three tiers —
the record forever, the lines for the window and then off-host, the genome blobs for the
horizon — in one set of terms.

## 27. A loss is written off in minutes, not in a day (`contract-b/4.1`, 2026-08-19)

**This set moves one number and adds no mechanism.** On 2026-08-19, reading the day's outage
backwash, the owner said:

> *"ok so i think we don't want the 24 hour thing it just forwards and if its lost is lost"*

**B42.** `forwardTimeoutMs`'s default becomes **300 000 ms (5 minutes)**. It was 86 400 000 ms
(24 hours) — the value `holdTimeoutMs` carried into §25, kept when B37 turned the deadline from
a bounce into bookkeeping. Everything §9.3 says still holds, word for word: a `sent` entry is
resolved by an answer or by this deadline, **nothing is re-sent at it and nothing comes home**,
the entry becomes a `lost` tombstone that still recognises a late `MIGRATION_ACK`, and
`lostForwardTotal` counts it where the map can read it.

**Why the number moved.** §9.3's open `sent` entries occupy the sender's outbound custody, and
contract A's export intake refuses new `MIGRATE_OUT` when pending outbound custody reaches
`inboundQueueMax` (64). On 2026-08-19 three overlapping outages — the cloud host's Spot
reclaim, five laptop worlds' dark window, and a broadcast-host reboot the night before — left
live worlds holding tens of forwards whose answers had died with dropped sessions. Under a
24-hour deadline those entries were immovable: senders sat at the intake cap refusing every
export, and an operator watching a world saw organisms cross a portal band and simply keep
walking. The deadline's length was doing no protective work — B37 already guaranteed that its
expiry moves no organism — so its only remaining effect was how long that wedge lasts. Five
minutes clears an outage's backwash in minutes while still sitting far above this map's
slowest honest answer (arrival pacing, save stalls and reconnect backoff together stay under a
minute); an answer slower than that lands on a tombstone and is still recognised.

**What it costs, named.** A destination that takes longer than five minutes to answer — a
paced arrival queue drained by a very slow world, or a peer that reconnects after a long
sleep — produces a `lost` record and a late-ACK log line for an organism that actually
arrived. The record is wrong in the safe direction: the ledger and `duplicatesRefused` still
reconcile the truth, and no organism is moved, duplicated or destroyed by the error. An
operator who wants the old patience back sets `--forward-timeout` / `MULTIVERSE_FORWARD_TIMEOUT`
— the knob §25 shipped — and owns the wedge that comes with it.

**Version.** No message, field, enum value, code, close code or routing input changes. A
sidecar with either default interoperates with every peer and with the relay unchanged; the
difference is observable only in its own journal, stats and logs. §4's test answers **neither
major nor minor**: the identifier **stays at `contract-b/4.1`** and `/contract-b/v4` does not
move, on §26's precedent.

**Enforced by:** the **sidecar**, whose compiled default this is (`internal/contractb`,
`ForwardTimeout`); the **operator**, for knowing that raising it re-arms the export wedge this
set exists to disarm; and **`--diagnose`**, whose `journal-depths` check is where a reader sees
the count and age of unanswered forwards this deadline governs.

## 28. Destination-paced transport and durable replies (`contract-b/4.1`, 2026-08-22)

**This set closes a capacity cascade without changing one frame.** A sidecar already paced its
own outbound backlog below the relay's published limits. That was necessary and not sufficient.
Several source peers could each stay below `maxFramesPerSecond` while they converged on one
destination. The destination then answered the combined arrivals on one connection.

The production-shaped result was a false `4007`: the destination produced more than 50 causal
ACK or NACK frames in one relay window. The relay closed it as over capacity. Bidirectional lanes
made the effect reinforce itself, and the map showed zero migrations per minute on healthy lanes.

**One amendment, B43**, covers the relay transport and the sidecar reply scheduler. The two
parts are one set. Relay pacing bounds immediate causal replies. Sidecar pacing bounds ordinary
durable ACK backlogs that can become ready later.

**This set does not change the wire.** No message type, field, enum value, NACK code, or close
code changes. No Contract B wire schema, routing input, URL path, or capacity field changes.
The existing values of `maxFramesPerSecond` and `maxBytesPerSecond` supply every bound.
§4's test therefore answers
neither major nor minor. The identifier stays at **`contract-b/4.1`**, and
`/contract-b/v4` does not move.

### B43 — Accepted migrations use bounded destination transport, and durable ACKs share the sidecar pace (§3.2, §3.3, §4, §5, §5.1, §5.2, §6.6, §6.7, §6.8, §6.12, §7.4, §9.1, §9.2, §11, §12, §14 B6, §20 B20, §22 B24, §22 B26, §25 B37)

**Gap.** B24 limited each peer's inbound frames. It did not limit aggregate migration fan-in
to one destination. Pacing only ordinary destination ACKs would leave immediate NACKs and
tombstone re-ACKs exposed. Pacing source handlers would stop their read loops and compress
unread control frames into the next capacity window. An asynchronous source job would retain
a decoded migration outside the transport and reopen duplicate and stale-routing hazards.

**Resolution.** The normative rules are integrated into the cited body sections. These are the
load-bearing properties.

| Property | Result |
|---|---|
| Bounded opaque queue | Each destination connection retains at most `F` migration frames and `B` migration bytes. The relay keeps the original bytes and reads no body. |
| Identity rate | Physical migration writes are evenly spaced at `R = max(1, floor(F / 8))`. The identity owns that pace across reconnect overlap. |
| Bounded control priority | Ordinary and control frames flow while no migration is due. After one becomes due, at most seven ready ordinary or control frames pass it. The migration then reserves the next write. |
| Nonblocking source | The source handler performs final routing and queue admission without waiting for the destination pace. It creates no later source job. |
| Safe full queue | A full migration queue returns `NOT_FORWARDED` without creating or updating a forwarding record. It does not close the destination. B46 adds exact attempt correlation for the bounded source-side progress path (§31). |
| Attempted-write boundary | The relay first rejects a missing, empty, or invalid `migrationId` as malformed. For a valid frame, final target validation, forwarding-record insertion, and queue acceptance are atomic. Acceptance then triggers the best-effort receipt and archive fan-out. |
| No later decision | An accepted queued frame never transfers to a replacement, re-routes, or produces a later NACK. Disconnect or process exit can lose it, and the forwarding record keeps the conservative ambiguity. |
| Ordinary close | A connection error or replacement drains ordinary frames, but drops queued migrations promptly. At most one already-selected migration write can finish. |
| Graceful relay drain | The relay closes admission, then drains every ordinary and migration queue in parallel at the normal pace. One 18-second relay-wide deadline covers them all. Expiry force-closes connections and drops any remainder. |
| Durable ordinary ACK | After `MIGRATE_IN_ACK`, the sidecar durably records `ackedUpstream: false`. Ordinary ACKs share the payload pace, run before new payloads, and retry until local relay enqueue succeeds. |
| Immediate causal reply | A sidecar NACK and a tombstone duplicate re-ACK bypass the deferred class. Each answers one relay-paced arrival and stays charged to the connection budget. The re-ACK does not clear `ackedUpstream`. The ordinary durable retry clears it. |

**Why 18 seconds.** A connection retains at most `F` migration frames. For supported `F ≥ 8`,
the pace portion is at most 15 seconds. The maximum occurs at `F = 15`. Eighteen seconds is the
hard relay-wide boundary, not a promise that a blocked final write completes. At the deadline,
the relay force-closes that write and drops every remaining frame. All accepted forwarding records
remain conservative.

**Why this remains D1.** The relay retains bounded transport bytes. It does not gain custody,
parse a payload, index a genome, inspect a species, or decide again after admission. The queue is
the connection writer's work, not an organism state machine. All durable custody stays in the
sidecar journals.

**What this does not promise.** Queue acceptance is still an attempted write under Contract B's
conservative rule. It is not delivery and it is not lossless. A disconnect after acceptance can
lose the frame, and the relay does not move it elsewhere. That cost is the existing at-most-once
ambiguity, moved to a bounded local queue rather than hidden in a source wait.

**Enforced by the relay:** atomic admission, bounded destination queues, identity pacing,
bounded ordinary priority, and relay-wide graceful drain.

**Enforced by the receiving sidecar:** durable pending ACKs, shared pacing, ACK priority, and
immediate causal replies.

Every reviewer rejects a source-handler wait, an asynchronous decoded migration, or a later
queue reroute. Each is a different design.

## 29. Species names are portable labels, not lineage identities (`contract-b/4.1`, 2026-08-23)

The live Species tab exposed a derived-record defect. Its genealogy used the A34-normalized
species name as a permanent node identifier. That assumption is false. The game can reuse a name
for a later species. The archive then replaced the earlier parent edge with the later claim.

Read-only analysis of the current fold found 99 names with more than one parent claim. It found
cycles of 4 and 50 names. In one living sample, 24 of 26 ancestry walks reached a cycle. The view
guard cut those walks. Related living species then appeared as separate roots.

**B44** changes only the archive's derived genealogy and its durable fold. It does not change the
raw record or any wire.

### B44 — The genealogy uses bounded, immutable lineage instances (§5.1, §10, §10.1, §26 B39)

| Rule | Statement |
|---|---|
| **The portable name is not the node identity** | A lineage instance is one A34-normalized species name on one recorded parent path. The first instance can use the normalized name as its stable key. A later instance has a deterministic derived key. |
| **Ordered world evidence selects an instance** | A crossing binds its species instance to its source and destination slots at `recordedAt`. A child claim first uses the parent instance bound to the source slot. If none exists, it can use the only recorded instance of that parent name. Equal timestamps use the immutable instance key as a deterministic second ordering key. |
| **Compatible evidence reuses an instance** | A crossing reuses an instance only when the normalized name and the parent instance agree. An incompatible parent path creates another instance. It **MUST NOT** change the earlier instance. |
| **Parent-only evidence stays explicit** | A child can create a parent placeholder when no crossing of that parent exists. A later crossing can establish the placeholder's parent path. If the promotion closes a cycle, the archive creates a separate later instance. It does not attach the unsafe edge. |
| **Unknown is still a value** | A self-parent, an unusable parent name, a capacity refusal, or evidence that cannot select one live instance stays unresolved. The surface **MUST** label it. It **MUST NOT** report that state as no parent evidence. |
| **The state is bounded** | The archive keeps at most `131,072` lineage instances. It publishes `lineageOverflow` when it refuses another. It also publishes `lineageInstances`, `splitNames`, `unresolved`, `cycleGuard`, `walkCapped`, and `nodesCapped`. Zero overflow and zero guard hits are deployment gates. `splitNames` is diagnostic and is not a failure by itself. |
| **The two species aggregates remain separate** | `/api/species` keeps its normalized-name totals and newest parent annotation. The genealogy uses lineage instances. The live census remains the only source for abundance, population, eggs, and worlds. A split lineage row **MUST NOT** claim a name-level genome count or population trend as its own. |
| **Suppression uses the portable name** | A deny-list decision uses the normalized `nameKey`. Suppression also replaces that field on the served node. A derived instance key cannot bypass the deny list or expose the suppressed name. |
| **Roll-up format 3 is required** | Format 3 persists every lineage instance and its per-world binding times. Format 2 has only the mutable name aggregate. A format-3 archive **MUST** refuse to start while a format-2 sidecar remains in place. The recorded rebuild operation **MUST** prove that the raw record is complete. It then **MUST** preserve and move the old sidecar before the full replay. An ordinary tail replay cannot reconstruct retired order. |

**What this costs.** One displayed name can now have more than one living row. That is the record's
answer when separate worlds carry separate occurrences. A parent-only placeholder can also remain
unresolved until later evidence establishes its path. Both costs are visible and are safer than a
false connection.

**What this does not do.** It does not resolve a name against a game registry. It does not infer a
parent from a genome. It does not rewrite, deduplicate, or merge raw migration records. Species
resolution remains inside the importing mod. The flat species totals remain grouped by normalized
name, and the census remains the only abundance source.

**Version.** No Contract B message, wire field, enum, code, close code, schema, URL path, routing
input, or custody rule changes. A peer cannot observe the fold. §4 answers neither major nor minor.
The identifier stays at **`contract-b/4.1`**, and `/contract-b/v4` does not move. Contract A takes
no set and no version change.

**Enforced by the archive:** ordered binding, immutable edges, cycle refusal, bounds, durability,
and honest fields.

**Enforced by the operator:** the format-3 rebuild and the zero-overflow, zero-guard result.

## 30. Exact transport refusal makes bounded progress (`contract-b/4.1`, 2026-08-23)

Twelve public samples isolated a source-side wedge. Slot 7 was live, mod-connected, populated,
and open on all four edges. It reported positive lane rates for six 30-second samples. It then
reported zero on all four lanes for six samples while `custodyDepth` stayed at 64.

The sidecar diagnostic reported `pendingAckDepth: 0`, `pacedDepth: 0`, and
`unresolvedDepth: 0`. The journal was valid. Thus, the 64 entries were safe refused or pending
outbound entries. They were not sent entries, a durable ACK drain, or an arrival pacing queue.

**B45** gives that exact state one bounded progress rule. It also closes the local crash gap at
the socket enqueue boundary.

### B45 — Proven `NOT_FORWARDED` walks once, then bounces once (§3.3, §5.2, §6.8, §7.4, §9, §12, §28 B43)

| Rule | Statement |
|---|---|
| **The proof is exact** | ~~The NACK MUST carry migration-wide `neverForwarded: true`, a matching relay session, and no same-session receipt.~~ **Narrowed — §31, B46.** Queue refusal now requires attempt correlation. The migration-wide value alone cannot prove a mixed-history attempt. |
| **The first refusal is atomic** | In one durable journal update, the source records `refused`, the refused destination in the tried set, and the first refusal deadline. The deadline is the first refusal time plus `bounceTimeoutMs`. |
| **Progress follows the original axis** | The source keeps the `migrationId`, body, annex, `exitEdge`, geometry, velocity, heading, and original timestamp. It walks after the refused destination and selects the next live, mod-connected, compatible, untried slot on that axis. Only `destSlot` and the `reroute` block change. Since B46, the relay reads only `reroute.count` for refusal correlation. |
| **One destination, one try per chain** | The durable tried set includes relay transport refusals and peer-local refusals in the same chain. A reconnect, restart, or compaction MUST NOT remove a slot from it. |
| **One deadline** | A reroute, retry, reconnect, restart, or compaction MUST NOT restart the first deadline. If the current map cannot prove that all alternatives are exhausted, the source waits only until this deadline. |
| **The other bounds stay hard** | The source bounces when the compatible untried walk is exhausted, when `maxReroutes` is reached, or when the first deadline expires. A repeated NACK or custody tick MUST NOT create a second bounce. |
| **Send commitment precedes enqueue** | Encoding, connection checks, and pace admission happen first. The source then fsyncs `sent`, `relaySessionId`, and `sentAt`. Only then does it offer the prepared frame to the local socket writer. A crash or enqueue error after the commit leaves the entry `sent`. It MUST NOT retry or bounce. It can only receive an answer or become `lost` at `forwardTimeoutMs`. |

**Why the deadline is `bounceTimeoutMs`.** This chain has exact proof that custody did not move.
It is the same safe class as a never-sent entry with no lane. Twenty seconds bounds the full
attempt, not each destination. A per-destination deadline would multiply the wedge by the row or
column length.

**Why `NOT_FORWARDED` is separate from other relay proofs.** A full transport queue can remain
live and selected as the current effective neighbour. Sending the entry back to that same queue
does not make route progress. `SLOT_VACANT` and `PEER_OFFLINE` already cause the normal effective
neighbour selection. They do not start this tried-destination chain.

**What this costs.** The conservative pre-enqueue commit can record a migration as lost when the
local writer rejects it before any byte leaves. Retrying that entry would reopen the crash window:
the earlier process could have enqueued the frame before it died. At-most-once chooses possible
loss over possible duplication.

**Version.** No message, field, enum, code, close code, schema, URL path, or relay routing input
changes. The new journal fields are private sidecar state. §4 answers neither major nor minor.
The identifier stays at **`contract-b/4.1`**, and `/contract-b/v4` does not move. Contract A
takes no set and no version change.

**Enforced by the source sidecar:** proof validation, the durable update, same-axis selection,
the one deadline, the hard reroute bound, the pre-enqueue commit, and one terminal bounce.

## 31. Queue refusal proof identifies one attempt (`contract-b/4.2`, 2026-08-23)

B45 used the migration-wide `neverForwarded` value as proof for `NOT_FORWARDED`. That works for
the first attempt only. After any accepted attempt, the value stays false. A later queue can still
refuse a different destination before acceptance. A peer refusal between two transport refusals
has the same mixed-history shape.

The proof must therefore identify the rejected payload, not the migration's full history. B46
adds that correlation and preserves B45's bounded progress rule.

### B46 — Queue refusal is correlated to the current attempt (§3.3, §4, §5, §5.1, §5.2, §6.6, §6.8, §6.12, §7.4, §9, §12, §30 B45)

| Rule | Statement |
|---|---|
| **The wire addition is optional** | A queue-full relay `NOT_FORWARDED` adds `refusedAttempt: {destSlot, rerouteCount}`. `rerouteCount` is 0 when the payload has no `reroute` object. An older reader ignores it. An older relay omits it. |
| **The legacy value stays migration-wide** | The same NACK keeps `neverForwarded` and `relaySessionId` for 4.1 readers. `neverForwarded` can be false after an earlier accepted attempt. Its value does not override an exact 4.2 attempt proof. Its presence distinguishes a proof-bearing queue refusal from graceful drain. |
| **Only queue-full carries attempt proof** | A graceful-drain `NOT_FORWARDED` omits `refusedAttempt`, `neverForwarded`, and `relaySessionId`. Drain does not consume a destination, start a first-refusal deadline, re-route custody, or mass-bounce the map. |
| **The source matches current durable state** | The NACK MUST be relay-generated `NOT_FORWARDED`. Its session, destination, and reroute count MUST match the current journal attempt. The source MUST reject missing, malformed, stale, delayed, duplicate, or mismatched correlation. Rejection leaves the entry and its scheduler unchanged. |
| **A current receipt contradicts the proof** | A `FORWARD_RECEIPT` for the same relay session and destination keeps the entry `sent`. A receipt for an earlier destination or another session does not contradict this attempt. |
| **A peer NACK is also current-attempt data** | `sourcePeer` MUST identify the peer that holds the current destination slot. A delayed NACK from an earlier destination cannot refuse, delay, reroute, or bounce the current attempt. |
| **The refusal update is atomic and durable** | The source records `refused`, adds the current destination to the tried set, preserves or creates the one absolute deadline, and clears the old attempt's `relaySessionId` and `sentAt` in one update. Restart and compaction preserve that result. A pending alternate lists no stale loss deadline. |
| **Pre-4.2 refused journals recover conservatively** | A replayed `refused` entry can carry the older migration-wide relay proof with no first-refusal deadline. It also retains the prior attempt's session and `sentAt`. That proof permits one retry of the recorded destination. The durable send commit replaces both stale values. Only a later exact 4.2 queue refusal starts the distinct-destination deadline and tried-set walk. |
| **No stale clone can send** | Before a pending or refused entry becomes `sent`, the source refreshes its current journal state while holding the sidecar scheduler lock. A stale snapshot from `Create` cannot commit a second send after another custody tick advanced the entry. |
| **Correlation pairs cannot repeat** | A present `reroute.count` MUST be a positive integer. Each rewrite increments it. The durable tried set prevents a destination from repeating inside the bounded refusal chain. Therefore, each `(destSlot, rerouteCount)` pair is unique in that chain. A zero, negative, or malformed present count closes the source with `4003`. |
| **The relay reads one new value** | For `MIGRATION_PAYLOAD` only, the relay decodes `reroute.count` with `sourcePeer` and `destSlot`. It uses the count only to echo queue-refusal correlation. It does not route on it. `fromSlot`, `proof`, `atMs`, the body, lineage, and species remain opaque. |
| **The absolute deadline wins over slow preparation** | Encoding, connection checks, and pace admission happen before the send commit. The source then checks the first-refusal deadline again. If it expired during preparation, the source bounces once and enqueues nothing. Otherwise it durably records fresh current-attempt session and `sentAt` values before enqueue. |
| **The original bounds remain hard** | Migration id, body, axis, geometry, and original timestamp do not change. The first deadline and tried set never reset. Alternate exhaustion, `maxReroutes`, or deadline expiry bounces once. A sent attempt cannot bounce from the historical refusal deadline. |
| **Archive keys preserve the sequence** | A field-present queue-full `NOT_FORWARDED` deduplicates on `migrationId + code + destSlot + rerouteCount`. A legacy, drain, or other NACK keeps the existing `migrationId + code` key. Live ingest and replay use the same rule. |

**Mixed-version behavior and rollout.** A 4.1 source ignores `refusedAttempt` and continues to use
the conservative migration-wide rule. A 4.2 source connected to a 4.1 relay receives no attempt
object and leaves a mixed-history queue refusal `sent`. This is safe degradation. The bounded
queue-refusal progress feature is unavailable until the relay is 4.2. Deploy the relay first,
then sidecars. Do not raise `minContractVersion`; this feature does not require a fleet eviction.

**Version.** `refusedAttempt` is an additive OPTIONAL field on an existing message. The relay's
payload projection reads one additive nested field. No type, enum, code, close code, or path is
removed or replaced. §4 answers minor. The identifier moves to **`contract-b/4.2`**, and
`/contract-b/v4` does not move. Contract A takes no set and no version change.

**Enforced by the relay:** positive count validation, queue-only echo, proof-free drain, atomic
queue refusal, and byte-identical forwarding.

**Enforced by the source sidecar:** current-attempt matching, receipt contradiction, durable state
refresh, one deadline, no repeated destination, pre-enqueue commit, and one terminal bounce.

**Enforced by the archive:** field persistence and identical attempt-aware live and replay keys.
