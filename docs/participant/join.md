# Join

The Windows and Linux installers join the public map automatically. A private map uses an
operator-issued join string. Both paths produce the same two local values: one public world
identity and one secret that only this installation must hold.

This page explains both paths, what happens on the first claim, and what other people can see
about your world once it is there.

## Automatic public enrollment

All participant `0.3.11` packages contain `public-map.json`. This file is the public join
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
   identity and relay address into the start script. On Windows it writes them into the launcher's
   world profile as well. The secret is never written into either one.

The public identity is `public-` followed by the UUID without punctuation. It is not a secret and
will appear on the public map after the first claim.

**A retry does not create another identity.** If the request succeeds but the response is lost,
the protected pending record keeps the same UUID and secret. The relay treats the exact retry as
the same enrollment.

**Neither does installing again.** Enrollment happens for a data root with no world in it, and
nothing else: a repair, an upgrade, a changed game folder, or an install over an earlier one reads
the identity already in that folder and keeps it, saying *"reusing the map identity already in …"*.
The uninstall keeps `peer-secret.txt`, `data/peer-id` and `data/relay-url` unless you pass
`-RemoveWorldData` / `--remove-world-data`, so even removing the software and installing it again
is the same world on the same slot, on a private map as much as on the public one.

**An install writes over a `peer-secret.txt` only when something vouches for the world it belongs
to**: the install record this data root's own installer wrote, or a pending record carrying that
very secret. A claim that an ordinary text file makes — `data/peer-id`, a launcher profile, an old
start script — is enough to *keep* a world and never enough to *destroy* one. Whatever is replaced
is kept beside the new file as `peer-secret.txt.<utc>.old`, for you to delete once the world
connects. In every other case the file is left exactly as it is and the installer stops instead:
your copy of that secret is the only one there is.

**A join string that names a different identity is gated**, because that is two things at once. A
**slot handover** mints exactly that — a new identity with a fresh credential, bound to your old
slot and position — and it is the map's only credential recovery. A borrowed or mistyped join
string looks identical from here, and applying one would leave the world that owns this journal dark
on the map. So the installer refuses both by default and applies the handover under
`-ReplaceWorldIdentity` / `--replace-world-identity`, keeping the old name in
`data/peer-id.previous`.

**If the secret is gone but the name survives** — an uninstall from a release before this one
deleted `peer-secret.txt` and kept the journal — the installer stops and says so. Nothing recovers
a secret. Ask that world's operator for the handover above and install the join string it prints
with the same switch, or use the switch on its own to take a **new** identity with no place: that is
a second world on the map, and the old one stays dark until its operator releases it.

Enrollment has a total map limit and a per-network-address limit. If either limit is reached, the
installer stops with `INS-ENROLL` and keeps the pending identity for a safe retry. Existing worlds
remain connected when the operator disables new enrollment.

**Each extra world on one computer enrolls again.** On Windows,
`multiverse-launcher.exe profile create NAME` runs this same enrollment for the new world, so
that world takes another public identity and counts against both limits. Several worlds created in
quick succession can meet the per-address limit; the launcher prints the `Retry-After` value the
service returns, and its pending record makes the retry safe.

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

**The Windows launcher takes the same file for an extra private-map world**:
`multiverse-launcher.exe profile create NAME --join-string-file .\join.txt`. Add
`--relay-url wss://<relay-host>/contract-b/v4` **only** when the file holds the identity half on
its own — a whole `multiverse-join/1` line already carries the relay address, and the launcher
never takes one from the flag over one the file carries. On the public map the flag is refused
outright, because the packaged `public-map.json` is the address. Delete the join file afterwards;
the launcher tells you to and will not do it for you.

Each installer splits the two halves at the last dot. It checks the secret before it writes the
credential. It stores only the secret in `peer-secret.txt` under the data root. Windows applies
an ACL for your account. Linux creates the file with mode `0600` before it writes the secret.

The identity and relay address go into the generated start script as `--peer-id` and `--relay`,
and on Windows into the world's profile as `peerId` and `relayUrl`. Both are fixed once the world
exists: `profile set` refuses to change either, because a world's identity and the map it is on
are what its journal is custody for. **The secret is never in a script, a profile, a command line,
or a log.** Delete the join-string file
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
- the speed the world is running at, its queue depths, and how many of its organisms have been
  lost in transit;
- **the keeper handle you chose, if you set one** — the name you are known by on the map. It is
  published exactly as you typed it, to everyone;
- **the world name you chose, if you set one** — the label your world is shown under, published
  the same way.

**Those last two are the only things on that list you pick yourself, and nothing invents them
for you.** If you set neither, your world publishes neither: no account name, no computer name
and no file name is read off your machine to fill the gap, and your world simply appears
unnamed. They are also the two things on the list that are shown to people rather than to
software, so choose them the way you would choose a name in any public place — they are visible
to every other world on the map and to everyone the operator has authorised to watch it.

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

Organisms that were mid-crossing can take longer to arrive after a restart. The ones your
machine had not handed over yet wait in your journal and cross when the relay is back — they
are safe, and they can go around a world that is still missing. **One that was already handed
over is handed over once**, though: nothing re-sends it and nothing brings it home, so in the
rare case where a restart swallows one it is gone. Crossing between worlds is the one dangerous
thing an organism does here, and the map would rather lose one than hold two copies of it — a
lost creature reads as a natural death, and a duplicated one is a broken world.

An **archive** restart takes longer. The archive draws the status page, so the page is
unavailable during its restart. The map keeps running, and crossings continue.

Planned restarts are announced. Unplanned restarts get an explanation afterwards.

**The map's record disposition is written before the run starts.** Reminders begin 30 days
before the end. Your journal, saves, and genomes remain on your machine.

**The map keeps three things for three different times, and says which is which.**

- **For the whole run and beyond**, it keeps the record itself: every species that crossed, how
  often, when it first and last did, and which species it descended from. Nothing removes a
  species, a count, or a family link.
- **For 30 days on the server, and then off it for the rest of the run**, it keeps the individual
  crossing lines behind those counts — one line for each organism that crossed. After 30 days the
  line moves to off-server storage. The crossing is still counted and the family link is still
  there.
- **For about a week on the server, and then off it for the rest of the run**, it keeps the
  related genome files. After that the genome moves to the same off-server storage, and the map
  fetches it back when a family view needs it. An older organism stays in the record and its
  genome stays reachable — it is kept, not dropped. Your copy also remains on your machine.

The operator keeps a compressed copy away from the server — of the crossing lines, and of the
older genome files — for the length of the run, and each one only leaves the server once that copy
is confirmed. It is not a copy you can ask for a deletion from — see
[`leave.md`](leave.md), "What you cannot take with you".

**A crossing that never arrives never enters the record.** An organism is handed over once; if
the destination does not answer, it is lost, the map counts the loss, and there is nothing to
recover — the record is what happened, and a crossing that did not complete is not part of it.

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
