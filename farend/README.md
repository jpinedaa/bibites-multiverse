# The far end — ring slot 2

This bundle puts one Bibites world on a second Windows computer. That world is
slot 2 of the multiverse ring. The main computer runs slots 1 and 3, the relay
and the archive.

You do not need Visual Studio, .NET, Go or WSL on this computer. You need Steam,
The Bibites, and Windows PowerShell 5.1, which every Windows 10 and 11 has.

## Before you start

Get three things from the main computer:

1. This bundle, `farend-bundle.zip`.
2. The token file, `~/.multiverse-token`. Copy it as `token.txt`.
3. The name or the IP address of the main computer, for example `192.168.1.20`.

The owner of the main computer must also open TCP port 8790 in the firewall
there. Without that rule this computer cannot reach the relay.

## The three steps

Unpack `farend-bundle.zip` into a folder, for example `C:\bibites-farend`. Put
`token.txt` in the same folder. Then open PowerShell in that folder.

**1. Install.** Run this one time.

```powershell
.\setup-farend.ps1 -RelayHost 192.168.1.20 -TokenFile .\token.txt
```

The script finds Steam's copy of The Bibites, checks that the game is the
correct version, installs BepInEx and the plugin, stores the token, and writes
`start-slot2.ps1` and `stop-slot2.ps1`.

**2. Start.** Run this each time you join the ring.

```powershell
.\start-slot2.ps1
```

It starts the sidecar, waits for the relay to grant ring slot 2, and then starts
the game. The game loads the world `M3-Slot2` by itself. Leave both running.

**3. Stop.** Run this when the test is over.

```powershell
.\stop-slot2.ps1
```

That is all. There is nothing else to do on this computer.

## If something fails

`setup-farend.ps1` stops and says why. Two answers cover almost all cases:

- **"The game on this machine is NOT the build this bundle was made for."**
  Steam updated one of the two computers. Both computers must run the same game
  version. Tell the owner of the main computer, who re-syncs and makes a new
  bundle.
- **"The relay did not grant a ring slot."** `start-slot2.ps1` prints the four
  usual causes and the last lines of the sidecar log. The first cause is the
  firewall rule on the main computer.

Logs live in `%LOCALAPPDATA%\BibitesMultiverse\logs` (the sidecar) and in
`<game folder>\BepInEx\LogOutput.log` (the game and the plugin).

## What the bundle installs, and where

| What | Where |
|---|---|
| BepInEx 5.4.23.3 | the game folder |
| `BibitesMultiverse.dll` | `<game folder>\BepInEx\plugins` |
| the token | `%LOCALAPPDATA%\BibitesMultiverse\token.txt`, readable by you only |
| the journal, the peer id and the ring slot | `%LOCALAPPDATA%\BibitesMultiverse\data-slot-2` |
| the sidecar log | `%LOCALAPPDATA%\BibitesMultiverse\logs` |

Keep the `data-slot-2` folder. It is this computer's record of every organism it
holds. If you delete it, this computer becomes a stranger to the ring, it takes
a new slot, and the organisms it was holding are lost.
