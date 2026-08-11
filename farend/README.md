# The far end — map slot 6, position (2,1)

This bundle puts one Bibites world on a second Windows computer. That world is
**slot 6** of the multiverse map, in the top-right corner of a 3×2 grid. The main
computer runs the other five worlds, the relay and the archive.

Your world is a full member of the map, not a spare. Every one of its four edges
is a door that works **both ways**: it exports east, north, west and south, and
it receives on all four as well. Organisms leave your world and arrive in it
while it runs, on every side.

You do not need Visual Studio, .NET, Go or WSL on this computer. You need Steam,
The Bibites, and Windows PowerShell 5.1, which every Windows 10 and 11 has.

## What changed, if you set this up before

The map used to share **one** password between every computer. It is gone. Two
things replace it, and both of them are new files you need:

- **Your own credential.** A secret that belongs to *this* world and no other. It
  authenticates the name `slot-6` and nothing else, so nobody holding somebody
  else's copy can take your place on the map. The main computer's relay printed
  it once and cannot print it again.
- **A certificate.** The link to the relay is encrypted now. Your computer
  refuses to connect until it can check the relay's certificate, and to check it
  your computer has to trust the certificate authority that signed it. There is
  no switch that skips the check. That is on purpose.

Your world, your saves and your journal are untouched by any of this. Run the
installer again with the two new files and start as usual.

## Before you start

Get four things from the main computer:

1. This bundle, `farend-bundle.zip`. It is in the repository, at
   `farend/dist/farend-bundle.zip`.
2. `ca.crt` — the relay's certificate authority. Not a secret.
3. `peer-secret.txt` — the secret half of this world's join string. **This one is
   a secret.** Its first line is the part after the last dot of the join string:
   a join string reads `slot-6.<secret>`, and only `<secret>` goes in the file.
4. The name or the IP address of the main computer, for example `192.168.1.227`.
   It has to be the name the certificate was issued for; the main computer's
   operator knows which one that is.

The owner of the main computer must also open **TCP port 8795** in the firewall
there, and forward it into WSL. Without both, this computer cannot reach the
relay. The port moved from 8790 to 8795 in M4, so a rule left from the earlier
milestone is not enough.

## The three steps

Unpack `farend-bundle.zip` into a folder, for example `C:\bibites-farend`. Put
`ca.crt` and `peer-secret.txt` in the same folder. Then open PowerShell in that
folder.

Windows refuses to run an unsigned script under its default policy, and it marks
every file that came out of a downloaded zip. Two commands clear both, for this
PowerShell window only:

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
Get-ChildItem . | Unblock-File
```

**1. Install.** Run this one time.

```powershell
.\setup-farend.ps1 -RelayHost 192.168.1.227 -CaFile .\ca.crt `
    -PeerSecretFile .\peer-secret.txt
```

The script finds Steam's copy of The Bibites, checks that the game is the
correct version, installs BepInEx and the plugin, trusts the relay's certificate
authority, stores your credential, and writes `start-slot6.ps1` and
`stop-slot6.ps1`.

**About trusting the certificate**, because it is the one step that touches
something outside this game. The certificate goes into **your own** trust store,
`Cert:\CurrentUser\Root` — not the computer's. It needs no administrator rights
and it affects no other account. While it is there, programs running as you trust
anything that authority signs; it was made by the main computer's operator for
this one relay. Windows may ask you to confirm. You can undo it at any time, and
the installer prints the exact command to undo it with. If you would rather do
the import yourself, pass `-SkipCaImport` and the script prints the command and
installs everything else.

**2. Start.** Run this each time you join the map.

```powershell
.\start-slot6.ps1
```

It starts the sidecar, waits for the relay to grant slot 6 at position (2,1),
and then starts the game. **That order matters:** the sidecar creates the file
the game needs to talk to it, so the sidecar always goes first. The game loads
the world `M4-Slot6` by itself, and seeds it on the first start. Leave both
running. Add `-Headless` and the game runs with no window and nothing drawn; the
world itself is unchanged. The switch belongs to that one start, so leaving it
off the next time brings the window back.

**3. Stop.** Run this when the test is over.

```powershell
.\stop-slot6.ps1
```

That is all. Nothing on the main computer ever sends a command to this one.

## The one extra step the pacing test needs

The main computer's arrival-pacing test asks this world to go away for a while
and come back. It cannot do that by itself, so it asks you, and it waits.

```powershell
.\stop-slot6.ps1 -GameOnly     # the world goes down. The sidecar stays up.
# ... wait until the main computer says it is done accumulating ...
.\start-slot6.ps1 -GameOnly    # the world comes back and the backlog drains
```

**Leave the sidecar running between the two.** It keeps its place in the map and
keeps taking custody of every organism that arrives while the world is away.
That backlog is the whole point of the test: when the world returns, the
organisms are delivered a few at a time rather than all at once.

## If something fails

`setup-farend.ps1` stops and says why. `start-slot6.ps1` prints the last lines of
the sidecar log and five usual causes. These three cover almost every case:

- **"The game on this machine is NOT the build this bundle was made for."**
  Steam updated one of the two computers. Both computers must run the same game
  version. Tell the owner of the main computer, who re-syncs and makes a new
  bundle.
- **"the relay's TLS certificate did not verify".** The certificate authority is
  not trusted here yet. The start script prints the one command that imports it.
  Nothing else is wrong, and nothing on the map was lost while you sort it out.
- **"The relay did not grant a slot"** with **HTTP 401** in the log. The
  credential in `peer-secret.txt` is not the one the relay holds for `slot-6`.
  It cannot be reprinted — ask the owner of the main computer for a **slot
  handover**, which issues a fresh one for this world. Your world and your
  journal are not affected.

Logs live in `%LOCALAPPDATA%\BibitesMultiverse\logs` (the sidecar) and in
`<game folder>\BepInEx\LogOutput.log` (the game and the plugin). Neither log ever
contains your credential; the plugin writes only the *path* of the file it reads.

Two things in the game log are worth a look while the test runs, and both are
for your eyes only — the main computer cannot see this screen:

- `[M4-PORTAL]` lines, and the portal strips themselves. **All four edges draw
  two lanes each**: an outer cyan capture lane, where an organism leaves, and an
  inner amber arrival lane, where one comes in. Both appear on every edge,
  because every edge now works both ways.
- `[M4-SAVE] event=SAVED` lines. The world saves itself every 10 minutes.

You can force one organism out with the `F10` key in the game. Nothing requires
it; organisms cross on their own.

One species stays home on purpose. `Basic bibite` — the stock the worlds were
seeded with — never leaves, on any edge. It still lives in your world and it is
still counted on the map's species view; it just does not travel, so the
organisms that cross are the ones evolution actually produced. `[M4-D18]` lines
in the game log are that policy at work, and they are not errors.

## What the bundle installs, and where

| What | Where |
|---|---|
| BepInEx 5.4.23.3 | the game folder |
| `BibitesMultiverse.dll` | `<game folder>\BepInEx\plugins` |
| the relay's certificate authority | your own `Cert:\CurrentUser\Root`, with a copy kept at `%LOCALAPPDATA%\BibitesMultiverse\relay-ca.crt` |
| your credential | `%LOCALAPPDATA%\BibitesMultiverse\peer-secret.txt`, readable by you only |
| the journal, the peer id, the slot and the position | `%LOCALAPPDATA%\BibitesMultiverse\data-slot-6` |
| the file the game and the sidecar share | `%LOCALAPPDATA%\BibitesMultiverse\data-slot-6\contract-a.token`, made by the sidecar |
| the sidecar log | `%LOCALAPPDATA%\BibitesMultiverse\logs` |
| the world and its backups | the game's own `Savefiles` folder, as `M4-Slot6.zip` |

Keep the `data-slot-6` folder. It is this computer's record of every organism it
holds. If you delete it, this computer becomes a stranger to the map, it takes a
new slot, and the organisms it was holding are lost.
