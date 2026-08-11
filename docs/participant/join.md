# Join

Joining a map is one thing you receive and one thing you do with it. This page is about the
thing you receive — **the join string** — what happens the first time your world claims a
place, and what other people can see about your world once it is there.

## The join string

**The map's operator mints it, their relay prints it once, and they hand it to you out of
band.** There are no accounts, no email, no password and no reset flow. That is a deliberate
choice and it has a price, stated below.

It carries three things:

| Part | What it is |
|---|---|
| The relay address | Where your world dials. It is a `wss://` address — this wire is always encrypted, and a relay refuses a plain connection rather than quietly redirecting one |
| Your world's identity | A stable name for your world on this map. It is **not** a secret; it is on the map's public page |
| The secret | The other half. This one is the whole of your world's identity on this map |

**Treat the secret the way you would treat the only key to a door.** Do not paste a join string
into an issue, a chat, a screenshot or a log. No log in this system prints one, at any level or
in any prefix, and no diagnostic asks for one.

**If you lose it, there is no software recovery.** The only path back is to ask the operator to
hand your slot over to a new identity with a freshly minted credential. That path exists, it
already refuses to run while your old world is still connected, and it prints its consequences
before it acts — but it needs a person. The cost is bounded by the thing that makes it
recoverable at all: **your reservation never expires**, so the slot, the position and every
organism addressed to it are still there when the operator gets to it.

### What one looks like

The relay prints a block on its own console, once, and the operator hands you what is in it:

```
JOIN STRING for peer-your-world — printed ONCE. Hand it over out of band.
  relay       wss://<relay-host>/contract-b/v4
  peerId      peer-your-world
  secret      <32 random bytes, hex>
  grant       peer
  one line    multiverse-join/1 wss://<relay-host>/contract-b/v4 peer-your-world.<secret>

  This relay keeps a VERIFIER, not the secret: it cannot print this again.
  If it is lost, the only recovery is an operator slot handover by name
  (multiverse-relay --handover-slot <n>=<newPeerId>), which mints a fresh one.

  On peer-your-world's own machine:
      umask 077; printf '%s\n' '<secret>' > ~/.multiverse-peer-secret
      multiverse-sidecar --relay wss://<relay-host>/contract-b/v4 \
          --peer-id peer-your-world --credential-file ~/.multiverse-peer-secret
```

**The `one line` form is the whole of it in one string** — `multiverse-join/1`, the relay
address, and your identity and secret joined by a dot — and it exists so that a join string can
be carried in one message. The two halves split on the **last** dot, because an identity may
legally contain one.

**Two commands apply it**, and they are the two above: put the secret in a file only you can
read, then start your sidecar pointing at that file. **No flag anywhere takes the secret
literally**, on purpose — a value on a command line is in every process listing on the machine.
It travels to the relay on the connection's own authorization header and never inside a
message.

**"It cannot print this again" is literal.** The relay stores a salted hash it cannot reverse,
so there is nothing for an operator to look up and nothing for them to re-send. That is why
losing it costs a handover.

**The `grant` line says which role the credential is for**, and a participant's is always
`peer`. The three grants — peer, subscribe, admin — are disjoint: a credential that holds one
does not hold another, and presenting yours for a role it does not carry is refused rather than
quietly downgraded.

**The package may ask for the same two things somewhere else** — a settings file instead of a
command line. What does not change is the shape: an identity, and a secret in a file only you
can read.

> **SLOT — WP6 (the package).** Where a participant puts it during install, and how they change
> it later.

## What happens on your first claim

Four steps, and you can watch all four.

1. **Your sidecar dials the relay**, carrying the credential on the connection itself — never
   in a message. A message is logged, copied and forwarded; a credential is not a thing that
   may ride one.
2. **The relay answers with what it is.** Its version, the wire version, the map's current
   shape, how many places are taken, the capacity limits **it is actually running with**, and
   the minimum wire version it will admit. You get all of that before you have asked for
   anything, so that you can be built to respect a ceiling rather than discover it as a
   refusal. The limits come again on every status broadcast, so a map reconfigured under you is
   one you learn about without reconnecting. The taxonomy's §3.2 lists them and what crossing
   each one does; **the numbers that bind you are the ones your map sends here**, not the
   defaults printed there.
3. **Your sidecar claims a place.**
4. **The relay grants one**, and tells you its number and its position.

The grant carries a reason, and the reason is the useful part:

| Reason | What it means |
|---|---|
| `granted` | A new reservation. This is your first claim on this map |
| `reclaimed` | **You already had one.** The reservation is keyed on your world's identity, never on a connection, and it never expires — a world that comes back after two hours or two weeks lands exactly where it was, and its neighbours re-pair to it |
| `updated` | A repeat claim on a connection that already has one |

## Where you land

**You do not choose, and that is usually right.** If you express no preference, the relay
places you in the first empty position inside the map's current rectangle; if there is none, it
grows the shorter axis, which keeps the map close to square instead of stretching one axis
until routing has nothing to route around.

Two rules are worth knowing because they explain a placement that looks arbitrary:

- **Holes are filled before the map grows.** A newcomer that could extend an axis is placed
  into an existing gap instead. Under continuous joining and leaving, that is what stops the
  map from stretching every time somebody arrives.
- **A preference never evicts anybody.** If you ask for a position somebody holds, your
  preference is ignored and you are placed by the ordinary rules.

## What your lanes will be

Your world has four edges. Each one is a door that works both ways: it exports on that side and
receives on that side.

**An edge is open when there is a live world to hand an organism to along that axis** — not
necessarily your immediate neighbour, because the map routes around dark worlds. An edge that
reports no peer is not always a fault:

- On a map one row tall, the north and south edges stay closed **for the life of the map**.
  There is no second row to walk to. That is arithmetic, not a failure.
- On a map that is a single world — the normal opening state of every map — every edge is
  closed and nothing is refused.
- On an axis of exactly two, losing the other world closes that axis entirely, because there is
  no third place to route around to.

[diagnose.md](diagnose.md) says how to read your own edges, and the taxonomy's §4 lists every
reason an edge can report and who has to act on it.

## What joining publishes about your world

**Nothing on this wire is confidential**, and that is a rule rather than a circumstance. The
relay broadcasts a block about every world to every other world on the map and to every
subscriber the operator has authorised. Your block carries:

- your world's identity and its slot and position;
- its population and egg count, and its species census — the names as your player sees them;
- the mod version, the wire version, the game version;
- your save policy, your exclusion list and whether your world wraps;
- the speed the world is running at, and its queue depths.

That is a fairly complete profile of one world on your machine, and it is written down here so
that you know it before you join rather than after. **A subscriber sees nothing a peer does not
already see** — there is no subscriber-only field and no back channel — and the operator's own
announcement of the map states what a participant is agreeing to.

> **SLOT — WP3 (the hosted deployment).** The map's announced period, its restart policy —
> which restarts are routine and what one looks like from your side — and what happens to the
> map's records when the announced run ends. All three are things a participant is told
> **before** joining.

## When the map refuses you

Three refusals happen at the door, and each has a different actor:

| What you see | What it means | Who fixes it |
|---|---|---|
| Your credential is refused | Wrong, missing or malformed. Your sidecar says so once per attempt, naming the remedy, and holds its retries at the ceiling after five — a wrong secret looks like a world that never joins rather than a machine hammering the map. Re-apply the join string; if it is lost, ask for a handover | **you**, then the operator |
| Your wire version is below the map's minimum | Your build is older than the floor this map's operator published. **Upgrade from the published release** — nobody on the relay's side can push it to you | **you** |
| Your game version is incompatible with the map's | The map is on a different game build. Only the operator can see which build the map is on | the operator |

Full detail, including what your sidecar must never do when it is refused, is in the taxonomy's
§3.1 and §3.2.

## Next

[diagnose.md](diagnose.md) — how to read what the map thinks of your world, and what to do when
something is wrong.
