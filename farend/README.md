# Far-end Windows peer

This bundle installs one Windows world as the far-end peer of the M4 test map.
The packaged scripts use peer `slot-6` at position `(2,1)`.

The bundle does not contain a relay address, credential, or private certificate authority.
The map operator supplies those values through a private channel.

You do not need Visual Studio, .NET, Go, or WSL.
You need a supported Windows version, PowerShell 5.1, Steam, and The Bibites.

## What the bundle installs

The setup script installs these parts:

- BepInEx for the supported game build.
- `BibitesMultiverse.dll`.
- The Windows sidecar.
- A test relay certificate authority in the current-user trust store.
- The peer credential in a current-user protected file.
- Start and stop scripts for the packaged world.

The peer exports and receives on all four map edges.
Its world, saves, and journal remain on this computer.

## Files from the map operator

Get these items from the map operator:

1. `farend-bundle.zip`.
2. `ca.crt`, which signs the test relay certificate.
3. `peer-secret.txt`, which contains only the secret half of this peer credential.
4. The relay name or address that appears in the certificate.

`ca.crt` is public trust material.
`peer-secret.txt` is a secret and must use a private transfer channel.

Do not paste a complete join string into a public issue or chat.
Do not send the credential in a screenshot.

The relay name must match the certificate.
An IP address works only when the certificate includes that IP address.

## Check the bundle

Compare the received archive hash with the hash published by the map operator.
Stop if the values differ.

Unpack the archive into a new folder, such as `C:\bibites-farend`.
Put `ca.crt` and `peer-secret.txt` in that folder.

Open PowerShell in the folder.
If Windows blocks the downloaded files, unblock only this extracted bundle:

```powershell
Get-ChildItem . | Unblock-File
```

Use a process-only policy change when the local policy blocks unsigned scripts:

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
```

This policy change ends when you close that PowerShell window.

## Install

Run setup one time:

```powershell
.\setup-farend.ps1 -RelayHost <relay-host> -CaFile .\ca.crt `
    -PeerSecretFile .\peer-secret.txt
```

The setup script checks the supported game assembly before it installs the plugin.
It stores the credential for the current Windows user.

The script can import `ca.crt` into `Cert:\CurrentUser\Root`.
This store affects only the current Windows user.

Trusting a certificate authority permits it to sign certificates for that user.
Check the displayed subject, thumbprint, and expiry before approval.

If you want to import it separately, use `-SkipCaImport`.
The script prints the required import command.

Delete the transferred `peer-secret.txt` after setup succeeds.
The protected installed copy remains in `%LOCALAPPDATA%\BibitesMultiverse`.

## Start

Run this command each time you join the test map:

```powershell
.\start-slot6.ps1
```

The script starts the sidecar first.
It waits for the relay to grant the reserved slot.
Then it starts the game and loads the packaged world.

This order is required.
The sidecar creates the Contract A token file that the game plugin uses.

Add `-Headless` to start without a game window:

```powershell
.\start-slot6.ps1 -Headless
```

## Stop

Stop the peer with this command:

```powershell
.\stop-slot6.ps1
```

The generated stop script uses `Stop-Process -Force` for the game and sidecar.
It does not request a graceful game shutdown.

CAUTION: If you must keep unsaved world changes, do not stop the peer.
If recent changes must be kept, wait for the next `[M4-SAVE] event=SAVED` log entry.

The default save interval is 10 minutes.
Thus, a forced stop can lose almost 10 minutes of world changes.

The script does not delete the sidecar journal files.
No command on another computer controls this peer.

## Pacing test

The arrival-pacing fixture can stop only the game while the sidecar keeps custody:

```powershell
.\stop-slot6.ps1 -GameOnly
# Wait until the test operator asks for the return.
.\start-slot6.ps1 -GameOnly
```

Keep the sidecar active between these commands.
The sidecar stores arrivals and releases the backlog after the game returns.
The `-GameOnly` command also forces the game to stop.
The same save-loss window applies.

## Common failures

### Unsupported game build

The setup script stops when the local game assembly differs from the supported assembly.
Get a bundle for the installed game build.
Do not bypass the assembly check.

### TLS certificate failure

Make sure that the displayed authority thumbprint matches the operator value.
Make sure that the relay name matches the certificate.

Import the authority only after both checks pass.

### HTTP 401 or no slot grant

The installed credential does not match the verifier for `slot-6`.
The relay cannot recover a secret from its verifier.

Ask the map operator for a formal slot handover.
The handover creates a new credential and requires setup again.

### No game connection

Read these logs:

- `%LOCALAPPDATA%\BibitesMultiverse\logs` for the sidecar.
- `<game folder>\BepInEx\LogOutput.log` for the plugin.

The logs can contain credential file paths.
They must not contain the credential value.

## Installed paths

| Item | Location |
|---|---|
| BepInEx | The game folder. |
| Plugin | `<game folder>\BepInEx\plugins\BibitesMultiverse.dll`. |
| Relay authority copy | `%LOCALAPPDATA%\BibitesMultiverse\relay-ca.crt`. |
| Peer credential | `%LOCALAPPDATA%\BibitesMultiverse\peer-secret.txt`. |
| Sidecar state | `%LOCALAPPDATA%\BibitesMultiverse\data-slot-6`. |
| Contract A token | `%LOCALAPPDATA%\BibitesMultiverse\data-slot-6\contract-a.token`. |
| Sidecar logs | `%LOCALAPPDATA%\BibitesMultiverse\logs`. |
| World saves | The game `Savefiles` folder. |

Keep the sidecar state directory.
It contains the durable custody journal and peer identity.

Remove the test certificate authority after the map no longer uses it.
The setup script prints the current-user trust-store removal command.
