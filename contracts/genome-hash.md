# The Canonical Genome Projection and `genomeHash`

**Version:** `bb8-genome/1`
**Status:** implementation-ready for M3. New on 2026-08-03, from decision D11 and
`m3_considerations.md` Risk 9.
**Owner:** `bb8-schema` (Go, sidecar-side only — D4, D7).

`genomeHash` is the join key of `multiverse-archive` and the node identity of the lineage
graph (D11). It is only useful if **two independent implementations, on two machines, in
two languages, produce the same 64 hex characters for the same organism**. This document
is written so that they do. It fixes the projection, the byte-exact serialization of that
projection, and the hash over those bytes, and it ends with a worked example whose input
string and output digest can be pasted into a test.

The key words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, **REQUIRED**,
**RECOMMENDED**, **OPTIONAL** are used as defined in RFC 2119.

---

## 1. Scope, and who computes what

| Party | Uses the hash for | Computes it? |
|---|---|---|
| `bb8-schema` | The projection and the hash themselves | **Yes.** It is the only implementation. |
| `multiverse-sidecar` | The lineage annex on `MIGRATION_PAYLOAD`, and the key of its genome cache | Yes, through `bb8-schema` |
| `multiverse-archive` | Content-addressed genome storage, the lineage graph, `GENOME_REQUEST` | Yes, through `bb8-schema` |
| `bibites-mod` | **Nothing** | **No — forbidden.** D4 keeps the bb8 body opaque to the mod. The mod ships opaque parent blobs and the sidecar hashes them (`contract-a.md` §14, A12). |
| `species-catalog` (M6) | Content addressing | Later, through the same projection |

The hash covers **one organism's genome**. It never covers a migration, a message, or a
file. The unrelated `bb8.Hash` used by `contract-a.md` §5.3 step 2 to detect a repeated
`migrationId` with a different body is a plain SHA-256 of the whole payload string and is
**not** this hash; the two never appear in the same field.

---

## 2. Why a projection instead of hashing the `.bb8`

A `.bb8` written by `SaveSystem.SaveBibite` carries full live state — position, velocity,
health, energy, stomach contents, age, per-neuron activations (`m1_findings.md` §2.6).
Those change every tick. A hash over the whole file gives every organism a fresh identity
every tick and a lineage graph with no edges at all.

The projection therefore keeps **only what is inherited**: the gene array, the brain's
nodes, the brain's synapses, and the game version that produced them. Everything else is
dropped.

The projection is also exactly the **intersection of the two `.bb8` dialects**
(`m1_findings.md` §2.6):

| Dialect | Written by | Genes at | Nodes at | Node keys present |
|---|---|---|---|---|
| **saved** (`.bb8`) | `SaveSystem.SerializeBibite` | `$.genes.genes` | `$.brain.Nodes` | `Type`, `TypeName`, `Index`, `Inov`, `Desc`, `baseActivation`, `Value`, `LastInput`, `LastOutput` |
| **template** (`.bb8template`) | `BibiteTemplate.SaveState` → `Node.SaveForTemplate` | `$.genes` | `$.nodes` | `Type`, `Index`, `Inov`, `Desc`, `baseActivation` |

The projected node keys are the five the template keeps, so **a saved organism and a
template exported from that same organism hash identically**. That is a property, not a
coincidence: it is what lets a genome fetched from a community template be recognised as
the ancestor recorded from a migration.

---

## 3. Dialect detection

`bb8-schema` **MUST** select the source paths by the presence of the `body` key at the top
level, exactly as `SaveSystem.LoadBibiteOrEggFromData` keys off `egg`
(`m1_findings.md` §2.6):

| Test on the parsed root | Dialect | Genes | Nodes | Synapses |
|---|---|---|---|---|
| `body` present | saved | `$.genes.genes` | `$.brain.Nodes` | `$.brain.Synapses` |
| `body` absent | template | `$.genes` | `$.nodes` | `$.synapses` |

A blob that matches neither — no gene object at either path, or no node array at either
path — is **unhashable** (§8).

The version tag is `$.version` in both dialects. It is a string. When the blob carries no
`$.version`, the caller's authoritative `gameVersion` (Contract A §4.6) **MUST** be used
in its place; the projection never has an empty version.

---

## 4. What the projection contains

The projection is a JSON object with exactly five keys. Nothing else is ever added to it
inside `bb8-genome/1`.

| Projected key | JSON type | Source | Rule |
|---|---|---|---|
| `genes` | object | the dialect's gene object | Every key present in the source, unchanged. Values by the number rule of §5.4. |
| `nodes` | array of object | the dialect's node array | One entry per node, five keys each (below), sorted by §5.3. |
| `projection` | string | this document | Always the literal `"bb8-genome/1"`. |
| `synapses` | array of object | the dialect's synapse array | One entry per synapse, five keys each (below), sorted by §5.3. |
| `version` | string | `$.version`, else the authoritative `gameVersion` | Copied verbatim. Never normalized, never parsed. |

### 4.1 A node

| Projected key | JSON type | Source key | Absent in the source ⇒ |
|---|---|---|---|
| `baseActivation` | number rule §5.4 | `baseActivation` | `0.0` |
| `desc` | string | `Desc` | `""` |
| `index` | integer | `Index` | **unhashable** |
| `inov` | integer | `Inov` | `0` |
| `type` | integer | `Type`, else `TypeName` | **unhashable** |

`type` is the **ordinal** of `NEATBrain.NodeType`. The game writes it as an integer
(`JToken.FromObject` on an enum), but a hand-written or community file may carry the name
instead, and `TypeName` carries the name in the saved dialect. An implementation
**MUST** accept either and **MUST** emit the ordinal, using this table
(`NEATBrain.cs:73-87`):

| 0 | 1 | 2 | 3 | 4 | 5 | 6 |
|---|---|---|---|---|---|---|
| `Input` | `Sigmoid` | `Linear` | `TanH` | `Sine` | `ReLu` | `Gaussian` |

| 7 | 8 | 9 | 10 | 11 | 12 | 13 |
|---|---|---|---|---|---|---|
| `Latch` | `Differential` | `Abs` | `Mult` | `Integrator` | `Inhibitory` | `SoftLatch` |

An unknown ordinal is projected unchanged — a future game version may add one, and
refusing to hash it would be worse than hashing it. An unknown **name** is unhashable,
because guessing an ordinal for it would make two implementations disagree.

`TypeName` is deliberately **not** projected: it is redundant with `Type` and the template
dialect does not carry it.

### 4.2 A synapse

| Projected key | JSON type | Source key | Absent in the source ⇒ |
|---|---|---|---|
| `en` | bool | `En` | `true` |
| `inov` | integer | `Inov` | `0` |
| `nodeIn` | integer | `NodeIn` | **unhashable** |
| `nodeOut` | integer | `NodeOut` | **unhashable** |
| `weight` | string | `Weight` | **unhashable** |

`weight` is polymorphic in the wild: a number in every file the game writes, and a **gene
reference string** in some community tooling (`system_decomposition.md`, Contract C). The
projection keeps the distinction and removes the ambiguity by tagging both forms:

- a JSON number becomes the `f32:` form of §5.4;
- a JSON string `W` becomes `"ref:" + W`, verbatim after the prefix.

A disabled synapse (`en: false`) stays in the projection. It is inherited topology, and
`CopyBrain` carries it to the child.

### 4.3 What is excluded, and why

| Excluded | Where it lives | Why |
|---|---|---|
| `Value`, `LastInput`, `LastOutput` | saved-dialect nodes | Live neuron activations. They change every brain tick. |
| `NIn`, `NOut` | node struct | `[NonSerialized]`, derived from the synapse list, and it is still open whether Newtonsoft emits them at all (`m1_findings.md`, research table). Deriving identity from a field that may or may not exist is the one thing this document cannot allow. |
| `brain.isReady` | saved dialect | A readiness flag, not genome. |
| `body`, `clock`, `transform`, `rb2d` | saved dialect | Live state (`m1_findings.md` §2.6). |
| `genes.gen`, `genes.tag`, `genes.speciesID`, `genes.parent1`, `genes.parent2`, `genes.isMutant`, `genes.isRad`, `genes.isRain`, `genes.isKiller`, `genes.isSource` | the gene **wrapper** object, not the gene array | Individual identity and bookkeeping, not heritable values. `parent1`/`parent2` in particular are the *lineage*, and putting the lineage inside the identity would make the graph self-referential. |
| `desc` (top level), `name`, `isOfficial`, `nodeAnchors` | template dialect | Cosmetic, editor-only. |

Note the asymmetry inside `genes`: the projection takes the **inner** gene object
(`$.genes.genes` in the saved dialect), never the wrapper that holds `gen` and
`parent1`/`parent2`.

---

## 5. Canonical serialization

The projection is serialized to a byte string with **exactly one** legal form. An
implementation **MUST NOT** use a general-purpose JSON encoder unless it can prove the
encoder obeys every rule below; the recommended implementation writes the bytes directly.

### 5.1 Structure

- The output is one UTF-8 byte string. No BOM. **No trailing newline.**
- **No whitespace anywhere.** No space after `:`, none after `,`, no indentation, no line
  breaks.
- `null` **MUST NOT** appear. An absent source value uses the default in §4.1 or §4.2, or
  makes the genome unhashable. There is no third outcome.
- No key is ever omitted. Every node object has its five keys; every synapse object has
  its five keys; the root has its five keys.

### 5.2 Key order

Object keys are sorted **ascending by the byte sequence of their UTF-8 encoding**, using
unsigned byte comparison, shortest-first on a tie (`"index"` before `"indexed"`).

This is a byte comparison, not a locale collation, not a UTF-16 code-unit comparison, and
not a case-insensitive comparison. Gene names are ASCII today, so the three coincide
today, and the rule is written down so they still coincide the day one is not.

The resulting orders are fixed and can be hard-coded:

| Object | Key order |
|---|---|
| root | `genes`, `nodes`, `projection`, `synapses`, `version` |
| node | `baseActivation`, `desc`, `index`, `inov`, `type` |
| synapse | `en`, `inov`, `nodeIn`, `nodeOut`, `weight` |
| `genes` | sorted at run time — the gene names come from the blob |

### 5.3 Array order

The source arrays **MUST NOT** be trusted to be in a stable order. The game writes them in
its own internal order, and a mutation, an editor round trip or a community tool may
reorder them without changing the genome.

| Array | Sort key | Duplicate ⇒ |
|---|---|---|
| `nodes` | `index` ascending | **unhashable** — two nodes with one index make the order arbitrary |
| `synapses` | `nodeIn`, then `nodeOut`, then `inov`, all ascending | **unhashable** — a fully equal triple makes the order arbitrary |

`en` and `weight` are deliberately **not** part of the synapse sort key. A genome with two
synapses between the same pair and the same innovation number is malformed, and hiding
that behind a tie-break would let two implementations agree on a hash for a genome that
one of them should have rejected.

### 5.4 Numbers

**Integers** (`index`, `inov`, `nodeIn`, `nodeOut`, `type`) are JSON numbers written as
shortest decimal: an optional leading `-`, then digits with no leading zero (`0` is
`0`), no `+`, no decimal point, no exponent. The range is `int64`; a value outside it is
unhashable.

**Floats** (every gene value, `baseActivation`, and a numeric `weight`) are **not** written
as JSON numbers at all. Each one is written as the JSON **string**

```
"f32:" + <8 lowercase hex digits>
```

where the eight digits are the big-endian IEEE-754 **binary32** bit pattern of the value.
`1.0` is `"f32:3f800000"`; `-1.5` is `"f32:bfc00000"`; `0.3` is `"f32:3e99999a"`.

The conversion is:

1. Parse the source token into a binary32 with a **correctly rounded** decimal-to-float
   conversion (round-to-nearest, ties-to-even). Every mainstream standard library does
   this: Go `strconv.ParseFloat(s, 32)`, C# `float.Parse`, Java `Float.parseFloat`, Rust
   `str::parse::<f32>`.
2. Reject `NaN` and `±Inf`, including a finite decimal literal that overflows binary32
   (`1e39`). Contract A §4.1 already forbids them on the wire; here they are unhashable.
3. Normalize `-0.0` to `+0.0`, so both spell `"f32:00000000"`.
4. Take the raw 32 bits (`math.Float32bits`, `BitConverter.SingleToUInt32Bits`,
   `Float.floatToRawIntBits`, `f32::to_bits`) and print them big-endian as exactly eight
   lowercase hex digits, zero-padded.

#### Why this float rule, and why it is safe across languages

The values are C# `float` — 32-bit — in the game
(`BibiteGenes.genes` is `float[]`, `Node.baseActivation` and `Synaps.Weight` are `float`).
Newtonsoft writes them as decimal text. Anything that hashes decimal text has to answer
"which decimal text?", and that question has no portable answer:

| Candidate rule | Why it was rejected |
|---|---|
| Hash the source token as written | Newtonsoft's output is a game implementation detail. A re-serialization by any tool changes the bytes without changing the genome, and the template and saved dialects need not agree on it. |
| Shortest round-trip decimal (Ryū / Grisu, Go `'g'`, `-1` precision) | The *digits* are well defined, but the *formatting* is not. Go prints `1e+06`, C# prints `1E+06`, Python prints `1000000.0`; the exponent threshold, the exponent sign, the case and the trailing `.0` all differ by language. Every one of those is a different hash. Pinning them would mean re-specifying a printf. |
| Fixed decimal places (`%.9g`, `%.17g`) | 17 significant digits round-trip a float64 but say nothing about a float32 parsed through a float64, and the formatting ambiguity above is unchanged. Fewer digits is lossy, which erases small mutations — the exact failure Risk 9 names. |
| **`f32:` bit pattern (chosen)** | Removes formatting from the problem entirely. There is exactly one bit pattern, exactly one hex spelling of it, and the primitive that produces it is in every language's standard library. The only remaining requirement is a correctly rounded decimal→binary32 parse, which IEEE 754-2019 requires and every mainstream runtime provides. |

Two further properties earned it the place:

- **It is exact at the source width.** A gene is a `float`; hashing its 32 bits hashes the
  value the game actually holds. A float64 round trip through the wire cannot perturb it,
  because the parse lands on the same binary32 either way.
- **It is mutation-sensitive at one ULP.** `0.3` and the smallest float above it differ in
  the last hex digit (`3e99999a` vs `3e99999b`), so the child of a mutation can never
  collide with its parent — which is the second half of Risk 9. §7.2 shows this.

The cost is that the canonical string is not pleasant to read. That is acceptable: it is a
hash input, never an interchange format. An implementation **SHOULD** offer a debug dump
that prints the same projection with decimal numbers, clearly labelled as *not* the
canonical form.

### 5.5 Strings

`desc`, `version`, `projection` and a `ref:` weight are JSON strings with **minimal,
deterministic** escaping:

- Escape `"` as `\"` and `\` as `\\`.
- Escape the C0 control characters `U+0000`–`U+001F` — using `\b`, `\t`, `\n`, `\f`, `\r`
  where JSON defines a two-character form, and `\u00xx` with **lowercase** hex otherwise.
- Escape nothing else. Every other character, including every non-ASCII character, is
  emitted as raw UTF-8.

Two traps, both real:

- **Go's `encoding/json` escapes `<`, `>` and `&` as `\u003c`, `\u003e` and `\u0026` by
  default.** An implementation that uses it **MUST** call `Encoder.SetEscapeHTML(false)`.
  A node description containing `<` is enough to split the lineage graph otherwise.
- **Never escape non-ASCII.** A `\uXXXX` form and the raw UTF-8 form are the same string
  and different bytes.

Invalid UTF-8 in a source string is unhashable. Unicode normalization is **not** applied:
`bb8-schema` hashes the bytes it was given, because normalizing would let two genuinely
different descriptions collide and would require a Unicode table in every future
implementation.

### 5.6 Booleans

`true` and `false`, lowercase, unquoted. `en` is the only boolean in the projection.

---

## 6. The hash

```
genomeHash = "bb8-genome/1:sha256:" + lowercase_hex( SHA-256( canonical_bytes ) )
```

- The digest is SHA-256 (FIPS 180-4) over the canonical UTF-8 bytes of §5, with **no**
  length prefix, **no** salt and **no** trailing newline.
- The hex is 64 lowercase characters, `0-9a-f`. The full field is 84 characters.
- The label is part of the value. A consumer **MUST** compare whole strings, and **MUST**
  treat a hash with an unknown label prefix as an opaque foreign identifier rather than
  rejecting the record that carries it — that is how a future `bb8-genome/2` coexists in a
  store that already holds `bb8-genome/1` hashes.
- The hash is **never truncated**. A shortened prefix may be used in a log line or a file
  path shard, never in a field, a key or a comparison.

SHA-256 rather than SHA-1 or MD5 because collisions there are constructible and this hash
is a database key. Rather than BLAKE3 because SHA-256 is in every standard library the
project will plausibly meet, including C#'s, and this hash outlives the current language
choice.

The projection version appears **twice** on purpose: inside the hashed bytes (the
`projection` key), so a future projection change cannot produce the same digest for
different content, and in the label, so a stored hash says what produced it.

---

## 7. Worked example

### 7.1 The base case

Input blob — a saved-dialect `.bb8`, abridged to three genes, two nodes and one synapse so
that the canonical string fits on the page. Every rule of §4 and §5 is exercised: live
state to drop, a wrapper to skip, an out-of-order gene set, negative floats, a hidden node
at index 48.

```json
{
  "transform": { "position": [2000.0, 412.77], "rotation": 274.11, "scale": 0.9312 },
  "rb2d": { "px": 2000.0, "py": 412.77, "vx": 6.12, "vy": 0.44, "r": 274.11 },
  "genes": {
    "tag": "Cyan", "speciesID": 41, "isReady": true, "gen": 37,
    "parent1": -843827577,
    "genes": { "SizeRatio": 1.0, "ColorG": 0.5, "ColorB": 0.30000001192092896 }
  },
  "body": { "id": { "id": -1180911975 }, "health": 42.5 },
  "clock": { "tic": 9, "timeAlive": 812.5 },
  "brain": {
    "isReady": true,
    "Nodes": [
      { "Type": 3, "baseActivation": -0.25, "TypeName": "TanH", "Index": 48,
        "Inov": 117, "Desc": "Hidden1", "Value": 0.13, "LastInput": 0.9,
        "LastOutput": 0.13 },
      { "Type": 0, "baseActivation": 0.0, "TypeName": "Input", "Index": 0,
        "Inov": 0, "Desc": "Constant", "Value": 1.0, "LastInput": 0.0,
        "LastOutput": 1.0 }
    ],
    "Synapses": [
      { "Inov": 204, "NodeIn": 0, "NodeOut": 48, "Weight": -1.5, "En": true }
    ]
  },
  "version": "0.6.3.1",
  "desc": "a worked example"
}
```

Canonical string — **390 bytes**, one line, no trailing newline:

```
{"genes":{"ColorB":"f32:3e99999a","ColorG":"f32:3f000000","SizeRatio":"f32:3f800000"},"nodes":[{"baseActivation":"f32:00000000","desc":"Constant","index":0,"inov":0,"type":0},{"baseActivation":"f32:be800000","desc":"Hidden1","index":48,"inov":117,"type":3}],"projection":"bb8-genome/1","synapses":[{"en":true,"inov":204,"nodeIn":0,"nodeOut":48,"weight":"f32:bfc00000"}],"version":"0.6.3.1"}
```

Hash:

```
bb8-genome/1:sha256:1725de8f1b61ba91fbeea7c91c47d3060b6ff97afbb6dfc2fc4062879a8bee14
```

Check the pieces by hand if you like: the gene object is sorted `ColorB`, `ColorG`,
`SizeRatio`; the nodes are re-ordered by `index` even though the blob listed 48 first; the
node's `TypeName` and its three activations are gone; `gen`, `tag`, `speciesID` and
`parent1` are gone; `-0.25` is `be800000` and `-1.5` is `bfc00000`.

### 7.2 A one-ULP mutation

Change one gene from `0.3` to the next representable float above it — `0.3000001`, which
is `f32:3e99999d` — and change nothing else. The canonical string differs in one
character, at one position, and the digest is unrelated:

```
bb8-genome/1:sha256:43ea8315112301799622f3ab8906c2882e528502bc35424a209cd67bfdaec207
```

This is the Risk 9 requirement, met: a mutated child never inherits its parent's hash.

### 7.3 The same organism as a template

A `.bb8template` exported from the same organism carries the same gene values, the same
five node keys and the same synapse, at `$.genes`, `$.nodes` and `$.synapses`. §3 selects
the template paths, §4 projects the same values, and the canonical string is
**byte-identical** to §7.1 — therefore so is the hash. A conformance suite **MUST** contain
this case.

### 7.4 Conformance checklist

An implementation is conformant when it reproduces all of:

| # | Case | Expected |
|---|---|---|
| 1 | §7.1 input | the §7.1 canonical string and digest |
| 2 | §7.1 input with the two nodes and the gene keys reordered | unchanged digest |
| 3 | §7.3 template form of §7.1 | unchanged digest |
| 4 | §7.2 one-ULP mutation | the §7.2 digest |
| 5 | `"desc": "a<b"` on a node | raw `<` in the canonical string, **not** `\u003c` |
| 6 | a gene value of `-0.0` | `"f32:00000000"` |
| 7 | a gene value of `1e39` | unhashable, `ErrUnhashableGenome` |
| 8 | two nodes with `Index` 5 | unhashable, `ErrUnhashableGenome` |
| 9 | the same blob hashed on Windows and on Linux | equal digests |

Case 9 is `m3_considerations.md` WP2's cross-machine test. It is a conformance case, not a
platform test: nothing in this document may depend on line endings, locale, CPU byte
order, or the Go version.

---

## 8. When a genome cannot be hashed

`bb8-schema` returns `ErrUnhashableGenome` and **no hash at all**. It never returns a
partial hash, a placeholder, or the hash of a repaired projection.

| Cause | § |
|---|---|
| Neither dialect matches — no gene object or no node array at either path | §3 |
| A node has no `Index`, or a synapse has no `NodeIn` / `NodeOut` / `Weight` | §4.1, §4.2 |
| A node's `type` is a name that is not in the `NodeType` table | §4.1 |
| A float is `NaN`, `±Inf`, or overflows binary32 | §5.4 |
| An integer is outside `int64` | §5.4 |
| Two nodes share an `index`, or two synapses share `(nodeIn, nodeOut, inov)` | §5.3 |
| A string is not valid UTF-8 | §5.5 |

**Nothing in the migration path fails because of this.** The consequences are bounded and
already specified elsewhere:

- An unhashable **parent** blob is recorded as a gap in the lineage annex, with
  `gapReason: "blob_invalid"` (`contract-b-m3.md` §6.6). It is not an error, the migration
  proceeds, and D11 already treats a missing parent as normal.
- An unhashable **migrant** blob means the payload itself is broken, and the deep
  validation gate rejects it first with `INVALID_PAYLOAD` (`contract-a.md` §9.1). A
  payload that passes validation and then fails to hash is a `bb8-schema` defect and
  **MUST** be logged loudly.

---

## 9. Relationship to the deep bb8 validation (WP2)

This document is the **seed** of the deep-validation work that M2 deferred
(`m2_considerations.md`, *Carried to M3*; `m3_considerations.md` WP2). The two read the
same fields, and they are ordered:

```
parse → deep validation → projection → canonical bytes → SHA-256
```

Deep validation owns everything this document deliberately does not:

| Validation concern | Owned by |
|---|---|
| Gene bounds, NaN/Inf rejection, gene-set completeness | deep validation |
| Node index bounds (0–47 standard, 48+ hidden), the 48-node registry | deep validation |
| No input→input synapse, index bounds, synapse-count and blob-size caps | deep validation |
| Weight polymorphism *rules* (which gene names a `ref:` may name) | deep validation |
| The `Utility.Version` alpha quirk in version ordering | deep validation |
| Cross-version gene conversion | deep validation |
| Byte-exact identity of what survived all of the above | **this document** |

A hash is only computed for a blob that passed validation, so §8's list is a defence in
depth, not the first gate. Where the two documents describe the same field, the
projection's rules are the narrower ones and this document wins for hashing purposes only —
validation may reject something this document would happily hash.

---

## 10. Open items

1. **`desc` is in the hash.** Renaming a hidden node in the in-game brain editor changes
   the genome hash even though no gene and no synapse changed. `Desc` is copied by
   `FinalizeEditing` and is carried by both dialects, so it is stable through inheritance,
   and dropping it would make two structurally identical brains with different labels
   collide. If the editor turns out to rewrite descriptions on its own, this is the one
   field to drop — and dropping it is a `bb8-genome/2`.
2. **The version tag is in the hash.** One genome hashes differently under two game
   versions. That is deliberate: gene names and `NodeType` ordinals are version-scoped, so
   a version-independent hash needs the cross-version conversion layer that
   `bb8-schema` has not built yet. It is harmless in M3 because the relay rejects a
   version mismatch at connect (`contract-b-m3.md` §6.1), so one ring runs one version.
   A mixed-version ring would split the lineage graph at the version boundary, and the
   answer is conversion-then-hash, not a weaker hash.
3. **`Inov` is in the hash.** Two independently evolved but structurally identical brains
   hash differently, because their innovation numbers differ. This is correct for lineage —
   they *are* different genomes by descent — but it means the hash is not a structural
   similarity key. M6's species catalog will need a second, coarser key. That key is not
   this one.
4. **Identity is the genome, not the individual.** Two organisms with byte-identical
   projections share one hash. The archive's lineage graph is therefore a graph over
   genomes; `entityId` in the annex is what distinguishes two individuals that carry the
   same genome (`contract-b-m3.md` §6.6).
