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

## Before you start

Get three things from the main computer:

1. This bundle, `farend-bundle.zip`. It is in the repository, at
   `farend/dist/farend-bundle.zip`.
2. The token file, `~/.multiverse-token`. Copy it as `token.txt`.
3. The name or the IP address of the main computer, for example `192.168.1.227`.

The owner of the main computer must also open **TCP port 8795** in the firewall
there, and forward it into WSL. Without both, this computer cannot reach the
relay. The port moved from 8790 to 8795 in M4, so a rule left from the earlier
milestone is not enough.

## The three steps

Unpack `farend-bundle.zip` into a folder, for example `C:\bibites-farend`. Put
`token.txt` in the same folder. Then open PowerShell in that folder.

Windows refuses to run an unsigned script under its default policy, and it marks
every file that came out of a downloaded zip. Two commands clear both, for this
PowerShell window only:

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
Get-ChildItem . | Unblock-File
```

**1. Install.** Run this one time.

```powershell
.\setup-farend.ps1 -RelayHost 192.168.1.227 -TokenFile .\token.txt
```

The script finds Steam's copy of The Bibites, checks that the game is the
correct version, installs BepInEx and the plugin, stores the token, and writes
`start-slot6.ps1` and `stop-slot6.ps1`.

**2. Start.** Run this each time you join the map.

```powershell
.\start-slot6.ps1
```

It starts the sidecar, waits for the relay to grant slot 6 at position (2,1),
and then starts the game. The game loads the world `M4-Slot6` by itself, and
seeds it on the first start. Leave both running.

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

`setup-farend.ps1` stops and says why. Two answers cover almost all cases:

- **"The game on this machine is NOT the build this bundle was made for."**
  Steam updated one of the two computers. Both computers must run the same game
  version. Tell the owner of the main computer, who re-syncs and makes a new
  bundle.
- **"The relay did not grant a slot."** `start-slot6.ps1` prints the four usual
  causes and the last lines of the sidecar log. The first cause is the firewall
  rule and the port forward on the main computer, on TCP 8795.

Logs live in `%LOCALAPPDATA%\BibitesMultiverse\logs` (the sidecar) and in
`<game folder>\BepInEx\LogOutput.log` (the game and the plugin).

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
| the token | `%LOCALAPPDATA%\BibitesMultiverse\token.txt`, readable by you only |
| the journal, the peer id, the slot and the position | `%LOCALAPPDATA%\BibitesMultiverse\data-slot-6` |
| the sidecar log | `%LOCALAPPDATA%\BibitesMultiverse\logs` |
| the world and its backups | the game's own `Savefiles` folder, as `M4-Slot6.zip` |

Keep the `data-slot-6` folder. It is this computer's record of every organism it
holds. If you delete it, this computer becomes a stranger to the map, it takes a
new slot, and the organisms it was holding are lost.
