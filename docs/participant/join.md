# Join

The Windows and Linux installers join the public map automatically. A private map uses an
operator-issued join string. Both paths produce the same two local values: one public world
identity and one secret that only this installation must hold.

This page explains both paths, what happens on the first claim, and what other people can see
about your world once it is there.

## Automatic public enrollment

All participant `0.2.1` archives contain `public-map.json`. This file is the public join
configuration. It contains the deployed HTTPS enrollment address and WSS relay address. It does
not contain a world identity or secret.

If “include the join string” means that the installer must already know how to reach the public
map, the package does include it. A literal `multiverse-join/1` string is different: it contains
one world's identity and secret, so it cannot be shared by all installations.

During installation, it does this:

1. It creates a random secret and installation UUID on your computer.
2. It protects a pending record for your account. Windows uses an ACL. Linux uses mode `0600`.
3. It sends the UUID, secret, and release number to the public enrollment endpoint over HTTPS.
4. The relay stores a salted verifier and returns the derived public identity and relay address.
   It does not return or log the secret.
5. The installer stores the secret in `peer-secret.txt`, removes the pending record, and writes the
   identity and relay address into the start script.

The public identity is `public-` followed by the UUID without punctuation. It is not a secret and
will appear on the public map after the first claim.

**A retry does not create another identity.** If the request succeeds but the response is lost,
the protected pending record keeps the same UUID and secret. The relay treats the exact retry as
the same enrollment. A later repair or runtime change also reuses the installed identity.

Enrollment has a total map limit and a per-network-address limit. If either limit is reached, the
installer stops with `INS-ENROLL` and keeps the pending identity for a safe retry. Existing worlds
remain connected when the operator disables new enrollment.

## Private-map join strings

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
  relay       wss://bibitesmultiverse.com/contract-b/v4
  peerId      peer-your-world
  secret      <32 random bytes, hex>
  grant       peer
  one line    multiverse-join/1 wss://bibitesmultiverse.com/contract-b/v4 peer-your-world.<secret>

  This relay keeps a VERIFIER, not the secret: it cannot print this again.
  If it is lost, the only recovery is an operator slot handover by name
  (multiverse-relay --handover-slot <n>=<newPeerId>), which mints a fresh one.

  On peer-your-world's own machine:
      umask 077; printf '%s\n' '<secret>' > ~/.multiverse-peer-secret
      multiverse-sidecar --relay wss://bibitesmultiverse.com/contract-b/v4 \
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

**Both advanced installers apply a private join string.** On Windows, run
`.\Install-BibitesMultiverse.ps1 -JoinStringFile .\join.txt`. On Linux, run
`./install-bibites-multiverse.sh --join-string-file ./join.txt`. The file overrides the packaged
public-map configuration.

Each installer splits the two halves at the last dot. It checks the secret before it writes the
credential. It stores only the secret in `peer-secret.txt` under the data root. Windows applies
an ACL for your account. Linux creates the file with mode `0600` before it writes the secret.

The identity and relay address go into the generated start script as `--peer-id` and `--relay`.
**The secret is never in that script, a command line, or a log.** Delete the join-string file
after installation. The installer does not delete a file that you supplied.

**To change a private-map identity later** — a slot handover, or a move to another map — run the
advanced installer again with the new join-string file. It rewrites the credential file and the
start script; your world, saves, and journal are untouched. **Your own copy is the only copy**:
nothing can reprint the secret, so there is no "show it to me again" anywhere in the software.

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

## The map, its period, and its restarts

**The map runs from 2026-08-14 to 2026-11-14 — three months.** The period is stated before
anybody joins. It can be extended by announcement, and it will never be silently shortened.
What ends is the shared map. Your world is on your machine and is unaffected.

**Restarts are routine and they are short.** The relay restarts from time to time. It can
restart to apply a setting or take a security update. **You do not need to
do anything.** Your sidecar notices the disconnect, waits, reconnects, and reclaims your slot
at your coordinate with `reason: "reclaimed"`. Your slot, position, and credential are unchanged.
They live on disk, not in the running process.

Organisms that were mid-crossing can take longer to arrive after a restart. **They are not
lost.** Your machine holds and resends them. The hold is bounded at 24 hours.

An **archive** restart takes longer. The archive draws the status page, so the page is
unavailable during its restart. The map keeps running, and crossings continue.

Planned restarts are announced. Unplanned restarts get an explanation afterwards.

**The map's record disposition is written before the run starts.** Reminders begin 30 days
before the end. Your journal, saves, and genomes remain on your machine.

**The map deletes one type of file on a stated schedule.** It keeps the crossing record and
ancestry for the whole run and beyond. It keeps the related genome files for **30 days**. An
older organism remains in the record, but the map might no longer have its genome. Your copy
remains on your machine.

## When the map refuses you

Three refusals happen at the door, and each has a different actor:

| What you see | What it means | Who fixes it |
|---|---|---|
| Your credential is refused | Wrong, missing or malformed. Your sidecar says so once per attempt and holds its retries at the ceiling after five. The relay stores only a verifier, so a changed or lost final secret needs an operator handover on both public and private maps | **you**, then the operator |
| Your wire version is below the map's minimum | Your build is older than the floor this map's operator published. **Upgrade from the published release** — nobody on the relay's side can push it to you | **you** |
| Your game version is incompatible with the map's | The map is on a different game build. Read the live builds at `https://bibitesmultiverse.com/live`. The operator coordinates convergence | the operator |

Full detail, including what your sidecar must never do when it is refused, is in the taxonomy's
§3.1 and §3.2.

## Next

[diagnose.md](diagnose.md) — how to read what the map thinks of your world, and what to do when
something is wrong.
