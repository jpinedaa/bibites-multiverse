# The error taxonomy

**Every refusal this system can hand a participant, with the remedy and the name of whoever
must apply it.** That second half is the point of the document. This system's most likely
public failure is one where the machine that suffers is not the machine at fault, so a
taxonomy that lists causes and stops there sends a stranger looking for a problem that is not
on their computer (`m5_considerations.md`, DQ8).

**Status: WP7's spine, with WP2's, WP4's and WP6's texture landed and WP7's own slots closed.**
Every entry below is taken
from the wire as WP1 published it — `contracts/contract-a.md` at `contract-a/2.4` and
`contracts/contract-b-m4.md` at `contract-b/4.1` — and every wording, value and log line that
belongs to the credential and TLS work (WP2), to the capacity table, the admin path and the
A49/A50 halves (WP4), or to the package and its installer (WP6) is quoted from what those
packages ship. Entries whose exact wording,
value or exit code is invented by a package that has **not** landed still carry a **slot**: a
marked to-do with the package that owns it. A slot is never a blank. §9 collects them.

## How to read an entry

| Column | What it holds |
|---|---|
| **Id** | A stable handle. `--diagnose` prints it, a support conversation quotes it, and it does not change when a table is reordered. |
| **Symptom** | What the participant actually sees: a close code, a NACK code, a log line, a field on the status page. Not the cause. |
| **Meaning** | What happened, in one sentence. |
| **Remedy** | The action that ends it. Where there is no action, the entry says so rather than inventing one. |
| **Who acts** | One of three values, below. |

**The three actors, and nothing else is one:**

- **you** — the participant, at their own machine. Everything they can reach: their own
  configuration, their own files, their own disk, their own game.
- **operator** — whoever runs the relay and the archive. Everything on the map's side, plus
  the one thing no participant can do: **see both ends of a failure**. A participant cannot
  reach another participant. When a fix has to happen on somebody else's machine, the entry
  says *operator*, and the remedy names the message they have to carry.
- **nobody** — the system recovers on its own, or the state is normal and the correct action
  is to wait. An entry that says *nobody* is saying so deliberately; it is not a shrug.

**One rule holds over the whole document.** No entry here ever asks a participant to turn off
a security control, to pass an `--insecure-*` flag, to skip certificate verification, or to
mint, guess or borrow a credential. If any tool or document in this project asks for one of
those, that is the defect, and reporting it is the remedy.

**Sections.** §1 install, before anything dials. §2 the local wire, between the game's mod and
the sidecar on the same machine. §3 the map wire, between the sidecar and the relay. §4 closed
lanes. §5 failures with no code at all. §6 refusals an operator causes on purpose. §7 things
that look like refusals and are not. §8 what to send when you ask for help. §9 the slots.

**Two platforms, and the document says which entries are which.** This release supports the Steam
copy on Windows and the native Linux build from itch.io. Most entries are the same on both and are
written with the command spelled for each. The platform-only rows are marked — `INS-SETUP`,
`INS-MARKOFWEB`, `INS-EXECPOLICY`, and `INS-BUSY` on Windows, `INS-NOTEXECUTABLE` and
`INS-LINUXDEPS` on Linux, `LOCAL-HEADLESSSTOP` on Windows, and in §5 the pair that matters most:
**`LOCAL-STARVATION` on Windows
and `LOCAL-LOGSHRED` on Linux are the same act with opposite symptoms.** One loses an instance
loudly; the other keeps every instance and destroys the evidence quietly.

---

## 1. Install, before anything dials

`INS-*` entries happen on the participant's machine before any wire exists. **Every wording below
is quoted from what the package ships** — the installers, the uninstallers and the release page of
`0.3.0`.

**The release recommends one setup executable on Windows and one complete archive on Linux.** It
also carries advanced ZIP packages. One `SHA256SUMS` covers every published artifact. Each
embedded or extracted package has a `MANIFEST.sha256`. A complete manifest also covers the game.

The table marks each platform-only entry. Everything else applies to both platforms.

| Id | Symptom | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `INS-CHECKSUM` | `(Get-FileHash -Algorithm SHA256 .\bibites-multiverse-0.3.0-windows-x64-setup.exe).Hash` — or `sha256sum bibites-multiverse-0.3.0-linux-x64-complete.zip` — does not equal the release-page value. The internal check can also print *"…is not the published file"* | The download or embedded package is not the published artifact | Delete it and download again from the release page. If it fails twice, stop and report it. Do not run it | **you** |
| `INS-SETUP` **(Windows only)** | Setup shows *"Bibites Multiverse Setup could not open or complete the installer"*, **or its window stays on *"Installing. Keep this window open."* after the game has already opened and joined the map**, **or setup is still in Task Manager minutes after the success dialog was answered, with its unpacked payload still in `%TEMP%` and no size shown beside the entry in *Installed apps*** | In the first, setup did not unpack its package, load the GUI, or finish installation. In the second **the installation itself finished**: the window was waiting on the world it had just started — the sidecar and the game, which are meant to keep running — instead of on the installer it ran, so it never came back. In the third **the installation is complete and so are the icons and the entry**: all that is left is the size figure Windows shows beside that entry, and a setup that measured the whole data root to get it had to walk the world's journal — 145k files and 14.1 GB on one machine, 27.5 minutes of it, with nothing on screen to say so | Compare the setup SHA-256 with the release page. Download it again when the values differ. If they match and the message repeats, report `INS-SETUP`. **For a window that will not finish**: the world is installed and running, so close the window. What did not happen is the setup's own last work — the shortcuts, the entry in *Installed apps*, and clearing its unpacked payload out of `%TEMP%`. Take the newest setup from the release page and run it again: it keeps the world already here, and writes those. **For a setup that lingers after the success dialog**: the world is installed, the icons work and nothing is at risk — leave it, and the size figure and the emptying of `%TEMP%` follow when the measure ends. Ending it in Task Manager costs only that figure and leaves the payload in `%TEMP%\bibites-multiverse-setup`, which you can delete by hand. A setup carrying the fix measures the program folder and the managed game copies instead, which is a scan you do not see | **you** |
| `INS-MARKOFWEB` **(Windows only)** | Windows shows **Unknown publisher**, SmartScreen stops the setup or the launcher on its first run, or an advanced ZIP script is blocked | The community setup is not code-signed. Windows also marked the downloaded file as internet content | First compare its SHA-256 with the release page. Then run `Unblock-File .\bibites-multiverse-0.3.0-windows-x64-setup.exe`, or select **Unblock** in its properties. If SmartScreen remains, select **More info → Run anyway** only for the matching file. Apply the same checksum-first rule to an advanced ZIP. **`BibitesMultiverseLauncher.exe` and `multiverse-launcher.exe` are unsigned too**, but step 1 clears the download mark from every file the manifest names, after checking each against `MANIFEST.sha256`, and the copy into the application directory adds no new mark — so Windows should not ask again on the first icon click. If it does, or if an antivirus quarantines the launcher, it is this entry one step later, and the answer is the same | **you** |
| `INS-EXECPOLICY` **(Windows advanced ZIP only)** | *"…cannot be loaded because running scripts is disabled on this system"* | The advanced ZIP starts an unsigned script directly. The recommended setup starts its verified local copy with `RemoteSigned` | Use the single-file setup. For advanced use, run PowerShell 7 or set `RemoteSigned` for the current user. Undo that setting with `-ExecutionPolicy Undefined`. **Nothing here asks for `Bypass`** | **you** |
| `INS-NOTEXECUTABLE` **(Linux only)** | `./install-bibites-multiverse.sh` answers *permission denied* | The archive reached this machine without its mode bits — unpacked by a tool that drops them, or carried through a filesystem that has none | `bash install-bibites-multiverse.sh` runs it once, and **its own step 1 sets the bit on the rest**, after the checksum and by name. This is the Linux analogue of `INS-EXECPOLICY` in position only: nothing is being switched off, and no policy changes | **you** |
| `INS-LINUXDEPS` **(Linux only)** | Step 0 stops with *"this machine is missing a program this package needs. NOTHING was installed."* and lists them | One of `sha256sum`, `awk`, `unzip`, `file`, or `curl` is absent. BepInEx uses `file` to read the game architecture. The installer uses `curl` for public HTTPS enrollment | `sudo apt install unzip file curl` on Debian or Ubuntu. **This is not `INS-NOTOOLCHAIN`** — no compiler, SDK, or runtime is involved. The check runs before the installer changes the game | **you** |
| `INS-GAMEBUILD` | The installer stops after step 3, prints your platform, your `BibitesAssembly.dll` hash and the builds the release supports, and ends with *"the game build is not in the support matrix; NOTHING was installed."* | The support matrix has no mod and sidecar build for **this game version on this platform** (D22) | The refusal quotes the matrix's own words: *"This release supports one game build, and the game on this machine is not it… Two ways forward: wait for a release whose matrix lists your build, or put this machine on a build this matrix lists."* The matrix is [`support-matrix.md`](support-matrix.md), beside the release, and travels in **every** archive as `support-matrix.json`. **A row is keyed on (game version, platform)** and each installer looks up only its own platform's rows, so this also fires on the *right* game run from the *wrong* archive — and then one of the rows it prints will match your hash, which is the archive you wanted. **Nothing about the map can fix this**, and joining anyway is not an option either installer offers | **you** |
| `INS-GAMEPATH` | The Windows GUI says *"Game not found"*, or the advanced installer stops because it cannot find `The Bibites.exe` | The existing-game option did not find a supported game folder | Use the included portable game, or select the folder that contains `The Bibites.exe`. The GUI keeps the folder picker available after an automatic search fails | **you** |
| `INS-RUNTIME` | A portable-game selection includes `-GameDir`; or the installer says its managed runtime **was changed** and *"It was not overwritten"*; or it refuses to remove a folder because *"that path is not `<data root>\runtimes\<payload sha256>`"*; or the copy of the payload into the runtime failed verification | The included-game selection owns a versioned copy of the game below the data root, `<data root>\runtimes\<assembly sha256>`; an existing-game selection uses the chosen folder instead. **A managed runtime that is COMPLETE and DIFFERENT is a working game copy somebody changed on purpose**, and the installer does not overwrite one. The last two are the guard on the one recursive delete this installer does: a path that is not exactly one managed runtime folder is never removed, whatever a descriptor or a record says | Select the existing-game option to use a game path. For a **changed** managed runtime: keep it if those changes matter and install with a different `-DataRoot`; if they do not, delete that one folder and install again — `<data root>\runtimes\<sha>` holds this package's own copy of the game and nothing of yours. **An INCOMPLETE managed runtime is no longer this refusal**: the installer says what it found there — how many payload files are gone, how many differ, how many files the game and BepInEx left behind — removes that folder and stages the payload again, which is what heals the machines described in the row below | **you** |
| `INS-RUNTIME` **(fixed: the install → uninstall → install cycle)** | Install, install again over the same managed game copy, uninstall, install again — and the last one stops with *"The managed runtime at `<data root>\runtimes\<sha>` is incomplete (`changelog.txt` is missing). It was not overwritten."* That folder holds the whole BepInEx layer — 27 files, 2.37 MB on the machine this was measured on — and not one game file | **The SECOND install created it.** It found BepInEx already in the managed runtime, which is there because the install before it put it there, and recorded `bepInEx.installedByThisInstaller: false` with an empty file list. The uninstall then took its *"it was here before us, leave it whole"* branch for the framework — and for the log, the config and the cache that go with it — while removing the game payload it HAD recorded. The install after that found a game folder with no game in it, and named a file nothing could put back | **Install a release carrying this fix over it**: BepInEx in the managed runtime is recorded as this install's, an incomplete managed runtime is removed and staged again rather than refused, and the uninstall reclaims the whole managed game copy — residue included — once nothing it recorded is left in it. Deleting `<data root>\runtimes\<sha>` by hand is the same act and is safe for the same reason: your worlds are the game's own saves elsewhere, and this world's journal, logs and credential are OUTSIDE that folder, in the data root. **The Windows uninstall now keeps a ledger file**, `<data root>\logs\uninstall-<utc>.log` — `%TEMP%` when the run deletes that folder, and nothing at all under `-DryRun` — so a remedy that says to read it names something you can still open after the window an *Installed apps* uninstall runs in has closed. The Linux uninstall prints the same ledger into the terminal you ran it from | **you** |
| `INS-NOTOOLCHAIN` | The installer asks for a compiler, an SDK, or a runtime you do not have | This should not happen. WP6's bar is an install with no build toolchain | Report it. Do not install a toolchain to work around it | **you** |
| `INS-ENROLL` **(public map)** | Step 6 says that the public map did not create the identity, returned a different identity, or reached an enrollment limit. It says that the protected pending identity was kept | HTTPS enrollment failed, the response was invalid, or the hosted service reached its per-address or total automatic-enrollment limit | Run the same installer again. It reuses the same UUID and secret, so a retry cannot spend another identity. If it repeats, check the service page and report `INS-ENROLL`; only the operator can raise or reopen a service limit | **you**, then **operator** |
| `INS-ENROLL` **(a world is already here)** | Step 6 says *"holds a world's secret, and nothing in `<data root>` says which world it belongs to"*, says that it looked in the sidecar log itself and found no world named there, and prints two numbered ways on | `peer-secret.txt` survived in that data root, and every witness that names the world it belongs to is gone: `install-record.json`, a pending record, the launcher's `profiles\*.json`, an old `Start-Multiverse.ps1` in any of the folders the installer looks in, `data\peer-id`, **and the sidecar's own log, which the installer reads before it says this**. It will not overwrite that secret, because it is the only recoverable half of an identity the relay can never print again. **It only asks when a world really ran here**: a journal, a Contract A token, anything at all inside `<data root>\data`, or a `peer=` in a sidecar log. A secret in a data root no sidecar ever ran in belongs to no world, holds no place on the map, and is not this refusal — the installer renames it to `peer-secret.txt.<utc>.orphan`, says so, and installs | Either **name the world**: its identity — `public-…` — is in an older kit's `Start-Multiverse.ps1` (`$PeerId`) and `profiles\default.json` wherever that kit was unpacked, and on the map's own status page. Put it into `<data root>\data\peer-id` on one line and run setup again, which then reuses it. Or **start a new world**: rename `peer-secret.txt` to `peer-secret.txt.old`, or move the whole data root aside. The old world then keeps its place on the map with nobody in it until the operator releases it, which is [`leave.md`](participant/leave.md) §2 | **you**, then **operator** if you want the old place released |
| `INS-ENROLL` **(the log names more than one world)** | Step 6 says *"THIS INSTALLER READ THE SIDECAR LOG in `<data root>\logs` and it names more than one world, so it will not choose between them"*, and lists each identity with the log line it came from | The sidecar writes `peer=<identity>` on every line it logs, so its log is the last witness a data root keeps once the record, the profiles and the start script are gone. This folder has run **more than one** world, and no file left in it says which one it is now — a choice only the person who ran them can make | Put the one this folder is now into `<data root>\data\peer-id`, on one line, and run setup again: it is then the strongest claim in the folder and wins over the log. The last one listed is usually the world that ran last, and the map's status page shows which of them still holds a slot. With no secret left either, the same refusal applies to enrolling over their journal, and `-ReplaceWorldIdentity` / `--replace-world-identity` is what accepts that cost | **you**, then **operator** to release the ones you are leaving |
| `INS-ENROLL` **(a different identity)** | Step 6 says *"is the world X … and that join string is a different identity, Y"*, or that a pending enrollment in that data root belongs to another world | A join string, or a half-finished enrollment, names a different world from the one whose journal is in this data root. **A slot handover mints exactly this**: it rebinds your slot and position to a new identity with a fresh credential (`contract-b-m4.md` §7.5), and it is the map's only credential recovery. A mistyped or borrowed join string looks identical, and then two identities sit over one journal and the old world goes dark | If a handover gave you that identity, apply it here with `-ReplaceWorldIdentity` / `--replace-world-identity`: the journal, the slot and the position stay. If it did not, install the new world with its own `-DataRoot` / `--data-root`. **Nothing was changed** either way, the old name is kept in `data\peer-id.previous` when the switch is used, and any secret replaced is kept as `peer-secret.txt.<utc>.old`. A pending record that disagrees is deleted only when you are sure which world you want | **you**, then **operator** for a handover |
| `INS-ENROLL` **(the secret is gone, the name is not)** | Step 6 says *"is the world `public-…` … and its secret … is gone"*, prints that identity and two numbered ways on, and stops | An uninstall from a release before this one deleted `peer-secret.txt` and kept the journal, or the file was removed by hand. **Nothing recovers a secret** — the relay keeps a verifier — so the only ways on are a **slot handover**, which gives this world's place to a new identity, or a new identity with no place, and neither happens quietly | Write the identity down. Then either ask that world's **operator for a slot handover** — it keeps the slot, the position and everything addressed to it, and mints a new identity whose join string you install with `-JoinStringFile` **and** `-ReplaceWorldIdentity` — or take a plain new identity with `-ReplaceWorldIdentity` / `--replace-world-identity` alone, which is a **second place on the map** and leaves the old world dark until the operator releases it. The old name is kept in `<data root>\data\peer-id.previous`. Renaming `data\peer-id` aside has the same effect as the switch, from a file manager | **you**, then **operator** |
| `INS-ENROLL` **(nothing proves the world)** | A join string names the world the data root claims to be, with a different secret, and step 6 stops with *"is the only thing that says … and it is an ordinary text file that nothing binds to the secret"* | The only thing naming this world is `data\peer-id`, a launcher profile or an old start script — text a person can write, and the file this project's own refusal asks you to write. That is enough to **keep** a world, which changes nothing, and never enough to **overwrite** its secret | If the secret in that file is spent, rename `peer-secret.txt` to `peer-secret.txt.old` and run the installer again with the same join string. If it is the live one, run the installer with no join string at all and it is kept exactly as it is | **you** |
| `INS-ENROLL` **(a different map)** | `-RelayUrl` / `--relay-url` over a data root that already holds a world, and step 6 stops naming both addresses | A world does not move between maps: its slot, its position and its journal are on the one it joined, and the same identity and secret presented to another relay is either a different world or an authentication failure | Run it again without `-RelayUrl` to keep the world, or use `-DataRoot` / `--data-root` for a new world on the other map | **you** |
| `INS-ENROLL` **(which map?)** | Step 6 says *"holds the world X, and nothing here says which map it is on"* | The data root has an identity and a secret but no relay address: `install-record.json` is gone, `data\relay-url` is not there, and the identity is not one the packaged public map issues — a private-map world uninstalled by a release that did not keep the address | Put that map's `wss://` address into `<data root>\data\relay-url` on one line and run setup again, or run the installer from a console with `-JoinStringFile` / `--join-string-file` holding the operator's one line. **Nothing was changed** | **you**, then **operator** for the address |
| `INS-ENROLL` **(the credential cannot be read)** | Step 6 says *"holds this data root's world credential, and this account cannot read it"*, with the reason the platform gave | `peer-secret.txt` exists and this account cannot open it — installed by another account, or run elevated once so its ACL names that account instead | Run setup from the account that installed the world. If that account is gone, the secret is gone with it: ask the operator for a slot handover, or move the file aside for a new world. **Nothing was changed** | **you**, then **operator** |
| `INS-ENROLL` **(that is not a credential)** | Step 6 says *"has something in it that is not a map credential"* and names the length it found | `peer-secret.txt` holds something that is not a secret — a truncated restore, a pasted line, an editor's stray text. A secret is 32 to 256 printable characters with no spaces | If a world's secret belongs there, put it back whole. If it is something else, rename it to `peer-secret.txt.old` and run setup again. **The file is not overwritten**, because a file the installer cannot read is not a file it may destroy | **you** |
| `INS-JOINSTRING` | A private-map installer stops on the join string: not three parts, no usable dot, a secret outside 32–256 printable ASCII characters, or a `ws://` relay address | The value pasted is not the one-line join string the operator's relay printed | Ask for the `one line` from that block, verbatim. A `ws://` address is refused rather than accepted quietly: this wire is always `wss://` and no part of this software falls back. **Nothing was written** — the credential file is written after every one of these checks. Neither installer has an option that takes the secret on a command line, on both platforms for the same reason | **you** |
| `INS-START` **(Windows only)** | The GUI says installation succeeded but the installed world did not start | The files and credential were installed, but the start did not obtain a map place or open the processes | Read the last lines shown by the GUI. Use the **Bibites Multiverse** icon to retry: it opens the launcher window, where **Start** runs the selected world and the panel says what happened, with **Show details** carrying every line of it. `multiverse-launcher.exe status` says what is running, and `start --wait 120` gives a slow map claim more time. Advanced users can run `Start-Multiverse.ps1` from the application directory | **you**, or the actor named by the start refusal |
| `INS-BUSY` **(Windows only)** | **Step 0** stops with *"something of this installation is running, and Windows does not let anything replace a program while it runs."*, lists each program by name, process id and path, and exits with code **3** rather than the usual `1`. The same refusal one step later, worded *"…is held open, so Windows will not replace it"*, means something started **during** the install | Windows holds a program's own file open for as long as it runs, and the launcher, the console launcher, the sidecar and the mod are all files an install replaces. **This is what a re-install over a live world used to hit at step 9**: a raw `Copy-Item : The process cannot access the file … because it is being used by another process` naming a line number in the installer, after steps 1 to 8 had already replaced the mod in the game and settled this world's identity — and with the setup around it quitting before it wrote the shortcuts or the *Installed apps* entry | **Stop every world first**: choose *"Stop every world"* in the launcher's window, or run `multiverse-launcher.exe stop --all` — `BibitesMultiverseLauncher.exe stop --all` in an install whose launcher has no separate console program — or `.\Stop-Multiverse.ps1` for a world this install's own script started. **Every one of those is lossless**; ending a world in Task Manager is not. Then close the **Bibites Multiverse** window itself, and The Bibites if it is still open, and run setup again. **NOTHING WAS CHANGED**: step 0 asks before setup has even checked its own package. **Nothing is ever ended for you**, because a world ended rather than stopped loses everything it has simulated since its last save. The check is on this install's own folders — the application folder `-InstallRoot` names, and the game folder step 5 writes the mod into — so a second copy of the game, or another world's launcher elsewhere, does not block it; a process whose path this account cannot read decides nothing on its own, and what settles it then is whether that folder's own files are held open. **The Linux installer refuses the game half of this without an id**, for a reason the platform does not force on it: replacing a file the running game has mapped into memory can crash a world mid-save. The pair at the other end is `INS-UNINSTALL-BUSY` | **you** |
| `INS-UNINSTALL-BUSY` | The uninstall stops with *"The Bibites is running from…"* or *"This install's sidecar is running"* | On Windows the platform holds the plugin open while the game runs. **On Linux it does not, and the refusal is kept anyway**: removing a file the running game has already mapped into memory is a way to crash a world mid-save, so the Linux uninstall refuses for a reason the platform does not force on it | Close that game, or run `multiverse-launcher.exe stop --all` on Windows — `.\Stop-Multiverse.ps1` / `./stop-multiverse.sh` also work. **Nothing was removed.** Both checks are on this install's own paths, so a second copy of the game elsewhere does not block it. **On Windows that holds even when the other copy runs under an account this one cannot inspect**: a process path it may not read decides nothing on its own, and what settles it is whether the folder's own files — `The Bibites.exe`, the plugin, the sidecar — are held open. A held file is named in the refusal; nothing held lets the uninstall go on. On Linux the check reads `/proc`, so a game **another user** is running out of your game folder is a case it cannot see and says so | **you** |
| `INS-UNINSTALL-CHANGED` | The uninstall's ledger says *"CHANGED since the install, so it is left alone"* against a path | A file the installer put there is no longer the file it put there. **In the complete edition's managed game copy it also keeps the folder**: that copy is reclaimed whole only when nothing the install recorded is left in it, so one file still vouching for it keeps everything around it too | Nothing, usually — this is the uninstall refusing to delete somebody else's work. Remove it by hand if you know what changed it. **On Windows the whole ledger is kept in `<data root>\logs\uninstall-<utc>.log`** (`%TEMP%` under `-RemoveWorldData`, and a `-DryRun` writes no file at all), so the line naming that path outlives the window it was printed in | **you** |
| `INS-UNINSTALL-NORECORD` | *"No install record at …, so this script cannot tell what it put on this machine and refuses to guess"* | `install-record.json` is missing from the data directory | Pass `-DataRoot` / `--data-root` if you moved it. **Nothing was removed**, and the uninstall will not fall back to guessing at paths | **you** |

**Where each install keeps its own files**, because half the remedies above name a directory:

| | Windows | Linux |
|---|---|---|
| This install's data root | `%LOCALAPPDATA%\BibitesMultiverse` | `${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse` |
| This world's secret — **never printed; replaced only by a join string this data root's own install record vouches for, or under `-ReplaceWorldIdentity`, and then the old one is kept as `.<utc>.old`** | `<data root>\peer-secret.txt` | `<data root>/peer-secret.txt` |
| This world's identity and its map, beside its journal | `<data root>\data\peer-id`, `<data root>\data\relay-url` | `<data root>/data/peer-id`, `<data root>/data/relay-url` |
| Installed application | `%LOCALAPPDATA%\Programs\Bibites Multiverse` | The extracted package directory |
| Complete-edition game runtime — **inside the data root, and the launcher never writes in it**: it is the one game folder allowed to live under a world's own data root, and `profile delete --remove-world-data` leaves it where it is. **It is a cache of the package's own payload, keyed on the hash of the game assembly in it**, so the installer rebuilds one that is no longer a game and the uninstall reclaims it whole once nothing it recorded is left in it — nothing of yours is ever in there | `<data root>\runtimes\<assembly-sha256>` | `<data root>/runtimes/<assembly-sha256>` |
| Your worlds — **never touched** | `%USERPROFILE%\AppData\LocalLow\The Bibites\The Bibites` | `${XDG_CONFIG_HOME:-$HOME/.config}/unity3d/The Bibites/The Bibites` |
| The mod's log | `<game folder>\BepInEx\LogOutput.log` | `<game folder>/BepInEx/LogOutput.log` — **and read `LOCAL-LOGSHRED` in §5 before you trust it** |

**One thing the Linux installer never does, stated here because its absence is the news:** it
writes to no system trust store. A private map's certificate authority is trusted through
`SSL_CERT_FILE` in the generated start script, for that one process — nothing under `/etc/ssl` or
`/usr/local/share/ca-certificates` is written and `update-ca-certificates` is never run. So the
Linux uninstall has no trust store to clean and no `--keep-certificate` flag, and **the Windows
`Uninstall-BibitesMultiverse.ps1` half of `INS-UNINSTALL-*` that removes a certificate has no
Linux counterpart at all**. Deleting the copy and the start script is the whole of the removal.

---

## 2. The local wire — the game's mod and your sidecar

Two processes on the participant's own machine, speaking `contract-a/2.4` over loopback
(`contract-a.md` §2). Nothing here involves the map, and **no entry in this section can be
caused by another participant.** That is worth knowing before reading a line of it: if the
symptom is in §2, the cause is on this machine.

### 2.1 Refused before the socket exists

| Id | Symptom | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `A-401` | HTTP **401** on the sidecar's own port; the mod logs `contract A: 401 from ws://127.0.0.1:…/contract-a/v2 — token rejected`, and after five tries the backoff pins at 30 000 ms and it stops saying anything new. The sidecar logs `contract A: upgrade refused, bad bearer token from 127.0.0.1:… (n of 5)` | The mod and the sidecar are not reading the same token file. The bearer token on this wire binds **two processes on one machine** and is nothing to do with the map (`contract-a.md` §21, A47) | Point both processes at the same token file. To rotate it, delete the file and restart the sidecar — the mod re-reads it on its next dial, so a rotation costs one reconnect and never a game restart. **This is not the relay credential; do not paste one into the other** | **you** |

**Nothing about custody moves while `A-401` is failing**, and the log line says so on purpose.
A mod that cannot authenticate has a closed export set, and the sidecar holds its journal. The
failure costs migrations that have not happened yet and costs no organism that has.

### 2.2 The session closed — `contract-a.md` §2.1

A close ends the session; it is never a report about one message.

| Id | Symptom | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `A-1009` | Close `1009` | A frame exceeded 8 MiB | Treat exactly as `A-4003` | **you** (report) |
| `A-4000` | Close `4000`, and the mod does not reconnect | The mod and the sidecar are on different **major** contract-A versions. The retired path `/contract-a/v1` also answers this way, on purpose, so an old mod gets a defined error instead of a bare 404 | Install the mod and the sidecar from the same release. They ship as a matched pair | **you** |
| `A-4001` | Close `4001`, and the mod does not reconnect | The mod is configured with a `ringSlot` that is not the slot this sidecar holds. The field is optional and advisory; it exists to catch a mis-wired rig in one second instead of one hour | Remove the slot setting. A participant's install should not name a slot at all — the relay grants one and the sidecar holds it | **you** |
| `A-4002` | Close `4002`, and the mod does not reconnect | This sidecar has no payload-schema dialect for the game version the mod reported. One of the four version gates D22 keeps as named exceptions (`contract-a.md` §21, A48). **Inert in practice** — the allow-list is empty outside tests and empty means accept | Install the build the support matrix names for your game version | **you** |
| `A-4003` | Close `4003` | A frame was not valid JSON, or an envelope field was missing or the wrong type. **This is a defect in one of the two programs, not a setting** | Report it with both versions and the log line either side of the close. Nothing on this machine configures it away | **you** (report) |
| `A-4004` | Close `4004`, then a reconnect about a second later | The sidecar heard no heartbeat for 13 000 ms. **The ordinary cause is a world save**: the save blocks the same thread that composes the heartbeat | Usually none — the mod reconnects, the sidecar holds arrivals in its journal for the whole silence, and nothing is lost. If it happens on nearly every save, save less often. See `LOCAL-SAVESTALL` | **nobody** |
| `A-4005` | Close `4005` | One side is draining — a sidecar stop, a world unload, a game quit | Wait. The mod reconnects when a world loads again | **nobody** |
| `A-4006` | Close `4006` on the older connection | A newer mod connection took over. **This is the self-healing rule**: a crashed-and-restarted game must be able to come back | None. It is the normal shape of a game restart | **nobody** |
| `A-4007` | Close `4007 EXPORT_EDGES_UNUSABLE`, and the mod does not reconnect. The reason names the declared set and the map's shape, e.g. `exportEdges [N,S] has no usable edge on a 3x1 map (no column axis)` | Every edge this world declared for export lies on an axis the map does not have, so **no declared edge can ever carry an organism**. A configuration error on this machine and nowhere else (`contract-a.md` §21, A50) | Set `MULTIVERSE_EXPORT_EDGES` to include an edge on an axis the map has — `E` or `W` where the map has no column axis, `N` or `S` where it has no row axis — or unset it entirely for the shipped default of all four edges. The sidecar's log carries the remedy and the actor in the same line: *the operator of THIS machine. Nobody else can fix this and no other peer is affected*. **The mod does not reconnect on its own**, deliberately: it would re-read the same setting and reach the same answer | **you** |

**Two closes are told apart in the sidecar's log and nowhere else.** `A-4004` covers both the
heartbeat deadline and the transport ping deadline, because the mod's reaction is identical
and it must never have to tell them apart. A diagnosis does: the heartbeat path logs a
`silentFor`, the ping path logs an `err` (`contract-a.md` §2.1, §20 A45).

### 2.3 An export was refused — `MIGRATE_OUT_NACK`, `contract-a.md` §9.1

The organism is revived in its own world and nothing is lost. A **transient** code closes one
edge until the next edge update; a **permanent** one is about this organism or this pairing.

| Id | Class | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `AOUT-EDGE_CLOSED` | transient | That export edge has no live neighbour right now | Wait. See §4 for what a closed edge means and whether it will reopen | **nobody** |
| `AOUT-NO_ROUTE` | transient | The edge is open and the sidecar has no destination for it yet: no slot granted, or the walk along that axis found no deliverable slot | Wait. It resolves when the map does | **nobody** |
| `AOUT-SIM_SIZE_MISMATCH` | transient | Your world and the neighbour's disagree about simulation size, so the edge mapping is undefined | Your world's simulation size must match the map's. **You cannot see which side is the outlier from here** — the operator can see both, so ask | **operator**, then **you** |
| `AOUT-PEER_INCOMPATIBLE` | permanent | The neighbour is on a different game build. The routing walk skipped it; this and a `peer_incompatible` closed edge are the two names the mod meets it under (`contract-a.md` §21, A48) | The map converges when every machine is on the same game build. **Which side is behind is not visible from here.** The operator sees every peer's version at once and is the only party who can say who has to move | **operator** |
| `AOUT-KIND_UNSUPPORTED` | permanent | The organism is not a bibite | None. Nothing retries it | **nobody** |
| `AOUT-INVALID_PAYLOAD` | permanent | The serialized organism failed its schema check, or exceeded the payload cap | Report it with the logged blob size. It is a defect, not a setting | **you** (report) |
| `AOUT-DUPLICATE_MIGRATION_ID` | permanent | A migration id is journaled against a different payload. A mod defect | Report it. The log is loud on purpose | **you** (report) |
| `AOUT-RATE_LIMITED` | transient | The local outbound limit, or the remote's admission control | Wait for the stated retry delay | **nobody** |
| `AOUT-JOURNAL_FULL` | transient | The custody journal is at its size or entry limit | Free disk on the volume holding the sidecar's data directory. See `LOCAL-DISK` | **you** |
| `AOUT-JOURNAL_ERROR` | transient | A durable write failed. **Custody was not taken**, which is the system working — the organism stays where it is rather than vanishing into a file that did not get written | Check free disk and the data directory's permissions. This is the loudest disk symptom on this wire | **you** |
| `AOUT-MALFORMED_MESSAGE` | permanent | A field failed validation. A mod defect | Report it | **you** (report) |
| `AOUT-SHUTTING_DOWN` | transient | The sidecar is draining; the session closes with `A-4005` next | Wait | **nobody** |

**No code in this table is ever caused by the lineage annex or by a species name.** A missing,
oversized or unhashable parent blob is recorded as a gap and the migration proceeds; a species
block that fails its shape rules is stripped and forwarded without
(`contract-a.md` §9.1, §14 A12, §16 A30). **A gap now says which kind it is**: a parent the game
had already destroyed is recorded as `parent_gone`, and one the mod carried and dropped to fit
the frame is recorded as `blob_dropped_for_size` (`contract-a.md` §21, A49). Neither is an
error and neither refuses anything; the distinction exists so that a lineage with holes in it
can be read as *a size ceiling doing its job* rather than as one undifferentiated absence.

### 2.4 An arrival was refused — `MIGRATE_IN_NACK`, `contract-a.md` §9.2

The sidecar keeps custody. A **permanent** code means the entry is **held for a person** and
never re-delivered — it stays in the journal until somebody releases it.

| Id | Class | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `AIN-SIM_NOT_READY` | transient | No world loaded, still loading, or paused | Wait | **nobody** |
| `AIN-SIM_OVERLOADED` | transient | The world is above the mod's admission ceiling | Wait. If it never clears, the world is carrying more than it can take and that is a local decision | **nobody**, then **you** |
| `AIN-EDGE_CLOSED` | transient | The arrival edge is not one the mod **declared**. A declared edge that is merely closed for export still accepts arrivals | Declare all four edges — the shipped default — unless you deliberately want a wall on one side | **you** |
| `AIN-DESERIALIZE_FAILED` | permanent | The game's loader returned null. It swallows every exception, so **no detail exists**, by construction | List your held entries and release the one named, bouncing it home or dropping it. Report it: this is a known carried gap and the report is the only evidence it produces | **you** |
| `AIN-RELINK_FAILED` | permanent | The organism restored and the parent/child repair threw; the mod destroyed the half-restored organism before replying | As `AIN-DESERIALIZE_FAILED`. Report it — the log is loud because this is the gap firing | **you** |
| `AIN-VERSION_UNSUPPORTED` | permanent | The arriving organism was serialized by a different game build and this world refuses the risk. **The first of D22's four kept gates** (`contract-a.md` §21, A48) | Release the held entry. The condition itself ends when the map converges on one game build, and only the operator can see who is where | **operator**, then **you** for the held entry |
| `AIN-KIND_UNSUPPORTED` | permanent | Not a bibite | Release the held entry | **you** |
| `AIN-MALFORMED_MESSAGE` | permanent | A field failed validation. A sidecar defect | Report it, then release the held entry | **you** (report) |
| `AIN-SHUTTING_DOWN` | transient | The game is unloading or quitting | Wait. Delivery resumes after the next handshake | **nobody** |

**Listing and releasing a held entry** happens on the machine that owns the journal and nowhere
else, because the relay cannot enumerate journals that are on other people's computers
(`contract-b-m4.md` §7.5, §9.3). Two commands, and **the sidecar must be stopped for both**: the
journal is a single-writer file.

```
multiverse-sidecar --data-dir <path> --list-inflight [--dest-slot <n>]
multiverse-sidecar --data-dir <path> --release-inflight <migrationId> bounce|drop
```

The list prints, per entry, the migration id, the entity, the direction and status, the
destination slot and exit edge, the durable handoff state, and — for an outbound entry that
reached the relay — **when it was forwarded and how long is left before it is recorded lost**,
with the reason beside it: *it is NOT re-sent and does not come home: migration is
at-most-once*. An entry that was never written to a live relay connection says exactly that
instead, because that is the one that cannot have duplicated anything.

**The release prints the risk before it acts**, and waits for a typed `YES` unless `--yes` is
passed. The words are the release's own:

> An entry in handoff "sent" WAS written to a live relay connection, so the far sidecar may
> already hold custody of this organism. If it does, and it returns and replays its own journal
> after you bounce this one home, **THE MAP HOLDS TWO COPIES.**
>
> At-most-once now carries NO automatic exception: this sidecar never bounces a forwarded
> organism by itself, and an unanswered forward is recorded LOST rather than brought home (§9.3,
> §25 B37). **THIS COMMAND IS THE ONLY WAY LEFT TO DUPLICATE AN ORGANISM ON THIS MAP**, and you
> are the one firing it.
>
> An entry in handoff "pending" or "refused" was never handed to anybody, or was refused before
> custody moved. Bouncing one of those cannot duplicate anything.

A `drop` says the other half in its own line — *the organism is GONE from this map. D2 accepts
loss; it never accepts duplication, and a drop is a loss you chose.* Neither act is reversible,
which is why the warning comes first.

---

## 3. The map wire — your sidecar and the relay

`contract-b/4.1` over TLS (`contract-b-m4.md` §3), on the same `/contract-b/v4` path: the minor
bump changes no frame, and a map carries sidecars either side of it without anybody being
evicted for the difference. **This is where another participant's machine can become your
symptom**, and every entry that has that shape says so.

### 3.1 Refused before the socket exists

| Id | Symptom | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `B-401` | HTTP **401** with `WWW-Authenticate: Bearer`; no WebSocket is created. Your sidecar logs one loud error per attempt — `contract B: the relay refused THIS PEER'S CREDENTIAL with HTTP 401`, carrying the count so far, this world's peer id, the remedy and who must act — and after five in a row it holds at the maximum backoff and stops saying anything new | The relay refused this credential: missing, malformed, or wrong. **The line names this peer's own credential on purpose**: a refusal at the door is about the secret this machine presented and about nothing on the map | Re-apply the correct private-map join string if you still have it. The secret goes in the file `--credential-file` names, never on a command line. **If the final public or private secret is lost or changed, there is no software recovery** — ask the operator for a slot handover (`contract-b-m4.md` §22, B22, and §7.5) | **you**, then **operator** |
| `B-429` | HTTP **429** with `Retry-After: 5` and the body `too many simultaneous connections from this address`; no WebSocket is created | Too many simultaneous connections from this source address. **The only capacity limit answered with an HTTP status instead of a close code**, because it is decided before there is a WebSocket to close — the relay has the address before it has read a header. It is deliberately loose: one machine legitimately runs several worlds | Run fewer worlds from this address, or ask the operator to raise the knob. A `contract-b/4.0` relay defaults to 8; the number that binds you is the `maxConnectionsPerAddress` your map publishes — see the limit table under §3.2 | **you**, then **operator** |
| `B-426` | HTTP **426** with `Upgrade: TLS/1.2, HTTP/1.1` | You dialled `ws://` at a relay that only takes `wss://`. It is answered with a refusal and **never a redirect**, because a redirect to a scheme you did not ask for is how a downgrade goes unnoticed | Use the `wss://` URL from the public configuration or private join string | **you** |
| `B-TLS` | The connection fails with a certificate error naming the host, the presented name and the verification failure. It retries and keeps failing | The relay's certificate could not be verified against your platform's trust store. **There is no flag that skips this and there will not be one** | Check your machine's clock and its trust store first — a wrong clock fails a valid certificate. If both are right, it is the relay's certificate and no client-side action makes it safe | **you**, then **operator** |

**A refused sidecar waits for a person, and that is a rule rather than a habit.** It keeps its
journal, goes on delivering inbound entries to its own mod, and **must not** generate a fresh
identity, fall back to an unauthenticated connection, fall back to `ws://`, or try another
peer's credential. Generating a fresh identity is the worst of those: it strands the slot, the
journal's destination addresses and every organism addressed to it
(`contract-b-m4.md` §3.1).

### 3.2 The session closed — `contract-b-m4.md` §3.2

| Id | Symptom | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `B-1009` | Close `1009` | A frame over `maxFrameBytes` (8 MiB by default). **An oversize frame is `1009` and never `4007`**: the size ceiling is a framing rule and capacity is a rate, and the two are told apart by the close code so that a client knows whether to hold its backoff | Report it | **you** (report) |
| `B-4000` | Close `4000`, and the client does not reconnect until restarted | A different **major** contract version, or a connection on a retired path. The retired paths stay served, over TLS, precisely so this error is defined rather than a bare 404 | Install the release that speaks the map's current major | **you** |
| `B-4003a` | Close `4003`, reason naming a malformed frame or a first frame that was not a handshake | A defect in the sidecar | Report it | **you** (report) |
| `B-4003b` | Close `4003` `"peerId does not match the authenticated credential"`, **after** the connection was accepted. Nothing appears on any slot's `lastRefusal` | The credential names one identity and the handshake claimed another. **This is the map's central security property working**: the peer whose identity was borrowed is not touched, not closed, and not told, because nothing happened to it. On the relay's side one line is written — `relay: refusing a handshake whose peerId does not match its credential`, naming the credential's peer, the claimed peer and the role, and stating that the peer whose id was claimed is not told and is not touched — and that line is the operator's whole view of it | Run the same installer again. A public-map repair reuses its completed identity. For a private map, re-apply the correct join string | **you** |
| `B-4003c` | Close `4003` naming both game versions, and `lastRefusal` appears on your slot for every other peer to see | The relay refuses a peer whose game version is incompatible with the map's. **The fourth of D22's four kept gates**, kept by the owner's call of 2026-08-11 (`contract-b-m4.md` §6.1, §22 B31) | Read every live slot's `gameVersion` at **https://bibitesmultiverse.com/live** or in `/api/status`. The console is public, read-only, and available without a credential. The operator coordinates convergence | **operator** |
| `B-4003d` | Close `4003` `"protocolVersion … is below this relay's minimum …"`, and the same `lastRefusal` on your slot | Your build is older than the floor this map's operator published. It is a **compatibility** control and never a security one — it stops the honest and inconveniences nobody else | Upgrade from the published release. **Nobody on the relay's side can do it for you** — the release channel pushes nothing (D25) | **you** |
| `B-4003e` | Close `4003` naming a missing grant, e.g. `"credential for … does not carry the subscribe grant"` | You asked for a role your credential does not carry. The three grants — peer, subscribe, admin — are disjoint. Nothing appears on your slot's `lastRefusal`, because your *peer* connection was refused nothing | Connect with the role your credential carries, or ask the operator for the credential the role needs. A subscriber grant is issued deliberately and never requested on the wire | **you**, then **operator** |
| `B-4004` | Close `4004`, then a reconnect on the backoff ladder | The relay heard nothing from this connection inside its liveness timeout. Ordinarily the network | Wait. If it repeats all day, the path between this machine and the relay is the thing to look at | **nobody**, then **you** |
| `B-4005` | Close `4005`, then a normal reconnect | The relay is draining — a routine restart. What a routine restart looks like is part of the map's announced restart policy (D24) | Wait. Your reservation never expires and you return to your own slot and position | **nobody** |
| `B-4005-evicted` | Close `4005` — **the code a draining relay sends** — and then every reconnect refused the same way: `4005` again, no `401`, and nothing on your slot's `lastRefusal`. **The only thing that separates it from a routine restart is that it does not stop** | The operator evicted this peer for a stated period. **The refusal deliberately has no shape of its own.** An eviction is a *liveness* act — the map treats an evicted peer exactly as it treats a dark one — so nothing on the wire announces one, and the absence is pinned by a test (`TestAnEvictedPeerIsToldWhatADrainingRelayTellsItAndNothingElse`) rather than left to drift. **It releases nothing**: the reservation, the slot number and the position all survive, and the return is an ordinary reclaim | Wait first — a restart looks identical and clears itself. If the refusal outlives one, contact the operator; nothing at this machine lifts an eviction. **Do not read it as a credential problem**: an evicted peer is never answered `401`, because its credential is not what is being refused | **operator** |
| `B-4006` | Close `4006` on the older connection | A newer connection that **authenticated as the same identity** took over. On your own restart this is the normal, self-healing shape | If you did not restart anything, somebody else is using your credential. Ask the operator for a handover to a new identity with a freshly minted credential | **nobody**, or **operator** if unexplained |
| `B-4007` | Close `4007 CAPACITY`, and the reason names the limit, the value it is set to and the measurement that crossed it: `maxFramesPerSecond 50 exceeded (peak 412/s over 3s)`, `maxBytesPerSecond 4194304 exceeded (peak … B/s over …)`, `maxConnectionsPerPeer 2 exceeded (3 open for this peerId)`. Your sidecar logs `contract B: the relay SHED THIS CONNECTION for capacity (close 4007)` with that reason verbatim; two in a row and it holds at maximum backoff. The same text appears on your slot's `lastRefusal` as `capacity: …`, for every other peer to see | A published capacity limit was exceeded and the relay is shedding **this connection, never the map**. No other peer's traffic changes, no lane closes, and nothing in flight is dropped. Your world goes dark like any other dark world and its neighbours route around it | Read the limits your map publishes — at connect and on every status broadcast — and bring this world under them. If your peer is legitimately inside them, this is a defect worth reporting; if the map's limit is genuinely too low for honest traffic, the operator raises it, because **every limit is a knob** | **you** (report), or **operator** |

**The limits, and what crossing each one does to you.** A relay speaking `contract-b/4.0`
publishes the values **it is running with** in a `limits` object — on `HANDSHAKE_ACK` when you
connect, and on every `PEER_STATUS` after it, beside each world's stats block rather than
inside it. The numbers below are the defaults it ships with; **the ones your map publishes are
the ones that bind you**, because an operator may have turned any of them.

| Limit | Default | Crossing it |
|---|---|---|
| `maxConnectionsPerPeer` | `2` | Close `4007` on the extra connection. The second exists for the overlap while a reconnect replaces an old session (`B-4006`); a third is one world dialled twice |
| `maxConnectionsPerAddress` | `8` | HTTP **429** on the upgrade — `B-429`. It is the one limit decided **before a WebSocket exists**, so it is answered with a status and not with a close |
| `maxFramesPerSecond` | `50` | Close `4007` |
| `maxFrameBytes` | `8388608` (8 MiB) | Close **`1009`** — `B-1009`, never `4007` |
| `maxBytesPerSecond` | `4194304` (4 MiB/s) | Close `4007`. It is what stops a frame ceiling from being evaded with maximum frames |
| `maxClaimsPerMinute` | `12` | `granted: false, reason: "rate_limited"` — `BCLAIM-rate_limited`. **The connection stays up**; a claim storm is answered rather than closed |
| `maxGenomeRequestsPerMinute` | `30`, per requester per answering peer | `found: false, reason: "rate_limited"` — `BGEN-rate_limited`. An answer, not a refusal of the session |
| `maxSubscribers` | `4` | Bounds how many authorised subscribers the operator may connect at once. **A participant never meets it** |

**Read a reading against the ceiling it is measured on.** The limits and every world's stats
arrive in the same broadcast for exactly that reason: a rate is only meaningful beside the cap
it is counted against, and a limit nobody can read is a support conversation nobody can win.

### 3.3 A placement claim was refused — `contract-b-m4.md` §7.2

The claim is answered, the session stays up, and the sidecar keeps claiming.

| Id | Symptom | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `BCLAIM-rate_limited` | `granted: false, reason: "rate_limited"`, and the session stays up | More claims in a minute than `maxClaimsPerMinute` allows (12 by default; your map publishes what it runs with). **The relay deliberately refuses rather than closes**, because a claim storm is usually a peer whose measured time scale is wandering, and a refusal it can read beats a close it must recover from | Check this world's time scale — see `LOCAL-TIMESCALE`. The claim will be granted once the storm stops | **you** |
| `BCLAIM-role_has_no_slot` | `granted: false, reason: "role_has_no_slot"` | A read-only subscriber asked for a world's slot | Connect with the peer role. A subscriber never holds a slot | **you** |
| `BCLAIM-version_incompatible` | `granted: false, reason: "version_incompatible"`, and the refusal appears on the status broadcast | The same kept game-version gate as `B-4003c`, met at claim time instead of at connect | As `B-4003c` | **operator** |

### 3.4 A migration was refused — `MIGRATION_NACK`, `contract-b-m4.md` §6.8

**The organism is still the sender's.** A sidecar never sends one of these after it has taken
durable custody, which is what makes both the bounce-back and the re-route safe.

| Id | Class | Sent by | Meaning | Remedy | Who acts |
|---|---|---|---|---|---|
| `BMIG-SLOT_VACANT` | **permanent** | relay | The destination slot names no reservation at all — released, or never issued. **Slot numbers are never reused, so that world never returns** | None. Where the relay can prove it never handed this migration anywhere, the organism re-routes along the same axis, or comes home. Where it cannot, the entry was already forwarded once and is **recorded lost** at `forwardTimeoutMs` like any other unanswered forward: there was never anything to wait for, and a slot that will never answer resolves as a loss rather than as a bounce that might duplicate (`contract-b-m4.md` §9.3) | **nobody** |
| `BMIG-PEER_OFFLINE` | transient | relay | The reservation exists and its peer is dark right now | Wait. The frame reached nobody, which is the relay stating that no custody moved, so the entry **re-routes** to the next live world along that axis — or comes home after `bounceTimeoutMs` where that axis has no lane to offer. Nothing retries the dark world, and nothing accrues against it | **nobody** |
| `BMIG-NOT_FORWARDED` | transient | relay | The relay declined this one attempt — a full outbound queue, a failed write before any byte left, or a drain | Wait | **nobody** |
| `BMIG-PEER_UNKNOWN` | transient | relay | The named peer is not connected. Applies to the acknowledgement and genome messages, which route on identity rather than on a slot | Wait | **nobody** |
| `BMIG-NOT_A_MEMBER` | **permanent** | relay | A read-only subscriber tried to send a migration | Connect with the peer role | **you** |
| `BMIG-OVERLOADED` | transient | sidecar | The destination's inbound queue is full, or its paced backlog stopped draining | Wait. A backlog is read against the destination's own configured delivery rate, and both are published per world | **nobody** |
| `BMIG-SIM_SIZE_MISMATCH` | transient | sidecar | Two worlds disagree about simulation size | As `AOUT-SIM_SIZE_MISMATCH` | **operator**, then **you** |
| `BMIG-MOD_ABSENT` | transient | sidecar | **The destination world has no mod connected and its queue is full.** The sidecar is up and the game behind it is not participating | **This is the failure a participant cannot fix and cannot see the cause of.** The destination's own operator has to restart that game — see `LOCAL-CONFIGRACE` and `LOCAL-STARVATION`, which are the two ways it happens. The map operator is the only party who can see which world it is and tell them | **operator** |
| `BMIG-INVALID_PAYLOAD` | **permanent** | sidecar | The blob failed its schema check, or is over the payload cap | Report it | **you** (report) |
| `BMIG-KIND_UNSUPPORTED` | **permanent** | sidecar | Not a bibite | None | **nobody** |
| `BMIG-VERSION_UNSUPPORTED` | **permanent** | sidecar | The destination has no schema dialect for the payload's game version. The map-wire face of `AIN-VERSION_UNSUPPORTED` | As `B-4003c` | **operator** |
| `BMIG-MALFORMED_MESSAGE` | **permanent** | sidecar | A field failed validation | Report it | **you** (report) |
| `BMIG-SHUTTING_DOWN` | transient | either | The sender is draining | Wait | **nobody** |

**A relay refusal is a statement about one attempt.** Whether the migration as a whole was
ever handed anywhere is a separate, provable fact the relay attaches to its own refusals, and
it is scoped to one relay process — after a relay restart the relay honestly says it cannot
speak for the period before it started. With no proof, **the sender does nothing at all**: the
frame is not sent again, the entry is not re-routed and the organism is not brought home, and
at `forwardTimeoutMs` the entry is recorded **lost** (`contract-b-m4.md` §5.2, §6.8, §9.2,
§9.3). Silence is never proof, and this map spends an organism rather than guess with one.
**Nothing here is a participant action**; it is written down so that an unanswered forward that
does not move for a day reads as designed behaviour rather than as a stuck queue.

**A refusal, on the other hand, is a statement — and a stated refusal still gets a second
world.** A sender acts on a code only where the code says, in as many words, that nobody took
custody: the frame never left this machine; the relay proved it forwarded nothing for the whole
life of the entry; or the destination refused it before journaling anything, which §6.8 forbids
it to do afterwards. Those entries re-route along the same axis, up to four times, and then
bounce home. None of it can duplicate an organism, because the only party who could hold a
second copy has said it holds none. **The one knob here belongs to the operator of this
machine**: a negative `--max-reroutes` (`MULTIVERSE_MAX_REROUTES`) turns re-routing off
altogether, making a crossing a single hop and bouncing a refused organism home instead of
offering it to a second world (`contract-b-m4.md` §9.2, §25 B37).

### 3.5 A genome fetch was refused — `contract-b-m4.md` §6.10

| Id | Symptom | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `BGEN-unknown_hash` | `found: false, reason: "unknown_hash"` | **A normal answer, not an error.** The source may be offline, the parent may never have migrated, or the source restarted and lost its cache | None | **nobody** |
| `BGEN-rate_limited` | `found: false, reason: "rate_limited"` | Over `maxGenomeRequestsPerMinute`, which is counted per requester **per answering peer** (30 by default) | Wait for the stated delay. The limit is a published knob and is on the same `limits` object as the rest | **nobody** |
| `BGEN-peer_offline` | `found: false, reason: "peer_offline"` | Relay-generated: the peer being asked is dark | Wait, or ask another live peer | **nobody** |
| `BGEN-too_large` | `found: false, reason: "too_large"` | The genome exceeds the payload cap | Report it | **you** (report) |
| `BGEN-shutting_down` | `found: false, reason: "shutting_down"` | The answering peer is draining | Wait | **nobody** |
| `BGEN-HASHMISMATCH` | A loud line: the fetched genome's recomputed hash differs from the hash asked for, and the answer is discarded | Content addressing doing its job. A store that trusted the label instead of the bytes would silently poison every lineage query that touched it | Report it with both hashes. **Nothing about this is a configuration** | **you** (report) |

---

## 4. Closed lanes

An export edge reports one reason, and under route-around a closed edge means **no slot on
that axis is deliverable** — not that one neighbour is down. The reason is therefore an
aggregate, and `contract-b-m4.md` §8 fixes exactly which value wins.

| Id | Reason | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `LANE-peer_live` | `peer_live`, open | The lane is working | — | — |
| `LANE-no_peer` | `no_peer` | **The shrug, and it has three quite different causes**: there was no other position on that axis at all; every candidate is a hole or an offline peer; or the candidates disagreed about why. It is also what a peer with no granted slot reports on every edge | Read the map. On a one-row map the north and south edges stay closed with `no_peer` **for the life of the map**, and that is a map shape and not a fault. Otherwise wait, and if the axis has genuinely lost everybody the operator is the one who can see it | **nobody**, or **operator** |
| `LANE-peer_mod_absent` | `peer_mod_absent` | Every candidate peer on that axis is connected with **no game behind it** | The neighbours' worlds are not running. See `BMIG-MOD_ABSENT` — the operator is the only party who can see which world and tell its owner | **operator** |
| `LANE-peer_incompatible` | `peer_incompatible` | Every candidate is on a different game build. The map is partitioned along a version boundary — the accepted behaviour after a staggered game update | The partition ends when every machine is on the same build. Nothing at this machine shortens it | **operator** |
| `LANE-sim_size_mismatch` | `sim_size_mismatch` | Every candidate disagrees with this world about simulation size | As `AOUT-SIM_SIZE_MISMATCH` | **operator**, then **you** |
| `LANE-peer_unreachable` | `peer_unreachable` | **This sidecar's own relay connection is down.** Decided before the walk, so it is about this machine and not the map | Read §3.1 and §3.2 for why the connection is refused or closing | **you** |
| `LANE-peer_overloaded` | `peer_overloaded` | Every candidate is shedding | Wait | **nobody** |
| `LANE-admin_closed` | `admin_closed` | This edge was closed locally | Reopen it, if you closed it | **you** |
| `LANE-A50-partial` | A warning line, **once per session and once per edge**: this edge is declared, lies on an axis this map does not have, will stay closed for the life of this map shape, and no organism is affected | **Legal and unchanged, and not a fault.** A set with at least one usable edge is never refused; only a set with none is (`A-4007`), and a map with no axis at all — a lone first world — refuses nobody | None, and the line deliberately claims no remedy: the remedy would be a map that grows an axis, and that is nobody at this machine's to apply | **nobody** |

---

## 5. Failures with no code at all

**The hardest class, because there is no error to search for.** Every entry here was found the
expensive way on the project's own rig and is recorded in `dev_environment.md`; what follows
is the participant-facing form, not a copy of the runbook.

**The logs a packaged install writes, and the exact place it puts each one:**

| Log | On Windows | On Linux |
|---|---|---|
| The game and the mod | `<game folder>\BepInEx\LogOutput.log`, rotating to `.1`–`.4`. **Truncated on every launch**, so copy it before restarting the game if it holds the evidence | `<game folder>/BepInEx/LogOutput.log`, and **it never rotates**: there is no `.1`–`.4` on this platform, because the rotation is driven by a file lock Linux does not take. Truncated on every launch in the same way — and see `LOCAL-LOGSHRED` below before you trust its contents |
| The sidecar | `%LOCALAPPDATA%\BibitesMultiverse\logs\sidecar.log`, with `sidecar.log.out` beside it. **A world the launcher created keeps its own `logs\` folder under its own data root** — `multiverse-launcher.exe status --all` names each one, and the launcher window's **Open logs folder** opens the selected world's | `${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse/logs/sidecar.log`, with `sidecar.log.out` beside it. The Linux start script also keeps `logs/game.out` — what the game itself printed to the terminal, which BepInEx's log does not carry |
| The launcher | `<that world's data root>\logs\launcher.log`, one line per event, no secret in it | **Not in this release**: the Linux kit ships no launcher |

**Neither ever contains your credential.** The mod logs the *path* of the token file it reads and
never its contents; no log in this system prints a secret at any level.

**A healthy mod log says so in one line**, and it is the line to look for first — the plugin
writes it at startup, then a configuration summary:

```
[Info   :Bibites Multiverse] Bibites Multiverse 0.6.7 loaded — contract-a/2.4 client. …
[Info   :Bibites Multiverse] [M2] config: enabled=True exportEdges=[E,N,W,S] borderEdges=[E,N,W,S] …
```

| Id | Symptom | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `LOCAL-CONFIGRACE` | The game loads, the mod framework reports a clean start, and then **nothing**: the world never joins, and the map shows this slot live with no game behind it. In `LogOutput.log`, an `[M2]` line about the settings file — on mod `0.6.6` and earlier, at **error** level, `[M2] configuration failed — the multiverse client stays off:` with an exception; on `0.6.7` and later, at **warning** level, `[M2] config: the settings file could not be written after 5 attempts`, and the world carries on regardless | **Something else held `BepInEx/config/dev.multiverse.bibites.cfg` while the mod was writing it.** BepInEx rewrites that whole file from inside *every* `Bind` call, so an old mod opened fourteen windows in its first second and any holder of the path could land in one. **A second game instance is only one of the holders** — an anti-virus or a search indexer walking a freshly installed plugin folder is another, and that one needs no second instance at all: it was seen on a single fresh install, 2026-08-17, and it left the game sitting at the main menu while the installer, the launcher and the sidecar all reported success. **The symptom does not look like a configuration problem at all** | **Fixed in mod `0.6.7`, in two places.** The mod now suspends the per-entry save, binds everything, and writes the file once with five retries; a save that still fails is a warning and the world stays on the map, because every setting is in the game's environment rather than in that file. And the start path no longer reports success without checking: the launcher and both generated start scripts wait up to 120 s for `mod.connected` on the sidecar's own `/my-slot` endpoint and print a loud warning naming this code if it never arrives. **On an older mod, or if it somehow recurs: restart that one game instance** — `multiverse-launcher.exe stop NAME` then `start NAME`, or `Stop-Multiverse.ps1` then `Start-Multiverse.ps1` | **you** |
| `LOCAL-STARVATION` **(Windows)** | The same symptom with **no `configuration failed` anywhere** — and, in a packaged install, **no `Bibites Multiverse 0.6.7 loaded` line either**, in a `LogOutput.log` the running game is otherwise writing. The slot reports no export edges | The mod framework hands out five log files per game folder and then gives up — and an instance that gets no log file **does not merely lose its log: the mod never loads in it**. **The tell is an absence.** The mechanism is an exclusive file lock, which is why this is the Windows shape: `LOCAL-LOGSHRED` below is what the same act does on Linux | **Restart that one instance**, so its environment matches the others exactly. **The launcher refuses to start a sixth world from one game folder for this reason** and names this code when it does, so on Windows a launcher-managed machine meets a refusal instead of this failure. **The ceiling is counted from ONE installation's profiles.** A second installed copy of the launcher keeps its own `profiles\`, counts only the worlds in it, and can therefore pass five between them against one game folder and meet this failure anyway — one installation per machine is the supported shape. This is a several-worlds-on-one-machine failure; a participant running one world will not meet it. What a single-world install *can* see with the same absence is a plugin in the wrong folder or a BepInEx that never installed — check that `<game folder>\BepInEx\plugins\BibitesMultiverse.dll` is there, and re-run the installer if it is not | **you** |
| `LOCAL-LOGSHRED` **(Linux)** | **Nothing is wrong.** Every world runs, every one joins the map, every one saves. What is wrong is `LogOutput.log`: `file BepInEx/LogOutput.log` answers **`data`** rather than text, most of it is NUL bytes, and it holds one session header and a scattering of lines where it should hold one set per instance. Measured on the rehearsal: **227 402 bytes of which 200 557 were NUL**, one of five session headers, two of five `event=SAVED` lines | **More than one game instance is running out of one game folder, and they share that file.** Linux takes no exclusive lock, so all of them open the same log, each keeps its own file offset, and each truncates at launch. No rotated `LogOutput.log.1`–`.4` is ever created. **This is `LOCAL-STARVATION`'s inverted twin and it is worse to debug:** Windows loses one instance loudly, and the failure announces itself; Linux keeps every instance working and destroys the shared evidence window for all of them at once, silently, so the log is already gone when you first go looking for it | **One instance per game folder.** For a second world on the same machine, unpack a second copy of the game into its own folder and install into that — it costs disk and nothing else. **There is no per-instance log path to configure**: BepInEx 5's log file is `BepInEx/LogOutput.log` under the game folder it was loaded from, and moving it means moving the folder. Nothing is lost when this happens except the log, and no organism, save or slot is affected — but do not diagnose anything else from a shredded log, because what it says is not what happened | **you** |
| `LOCAL-TIMESCALE` | Every rate reading about this world is wrong by a large factor, and nothing else looks broken | The world is running at a speed nobody intended. The game restores its own speed after a world settles, which can land **after** a speed command; and **the first speed command after a world load reports `1.00` and sticks there** — a second one twenty seconds later takes | Read the applied speed **and** the achieved speed for this world, and set it again if it is wrong. At a speed the machine cannot meet, the two numbers come apart, and the gap is the news | **you** |
| `LOCAL-SAVESTALL` | The mod logs a save that exceeded its stall budget — an `[M4-SAVE]` line — and sometimes an `A-4004` close within a few seconds of it | A world save blocks the thread the heartbeat is composed on. The cost is a session churn and a short delivery pause — **arrivals are held in the journal for the whole silence, in order** — and not an organism | Usually none. If nearly every save does it, save less often: raise the save interval — `multiverse-launcher.exe profile set NAME --save-minutes 20` on Windows, or `MULTIVERSE_SAVE_MINUTES` in `start-multiverse.sh` on Linux. With several worlds on one computer, stagger their intervals. A longer interval with more retained saves (`MULTIVERSE_SAVE_KEEP`) is a deeper history, not a shallower one, and it makes stalls rarer rather than shorter. **A headless world wants the opposite** — see `LOCAL-HEADLESSSTOP` below | **nobody**, then **you** |
| `LOCAL-HEADLESSSTOP` **(Windows)** | A world that was running headless comes back missing everything since its last save. Nothing logged an error and the stop reported success. A world with a window, stopped the same way, does not lose anything. **The stop that did it said `forced immediately: it has no window`** — any other wording and this is not what happened, and that sentence is only ever printed about **the game**: the sidecar has no window either, and a healthy stop of one says `ended directly; a sidecar keeps nothing unsaved - its journal is written as it goes`, which is not this | **A headless game has no window, so there is no close request to post to it.** `-batchmode -nographics` is what removes it. `taskkill` without `/F` answers *"can only be terminated forcefully"*, and every Windows stop path — the launcher's `stop` and the generated `Stop-Multiverse.ps1` — then forces the process. **That used to be EVERY stop rather than only a headless one.** The launcher's ask carried `/T`, which walks the process tree, meets the windowless `UnityCrashHandler64.exe --attach <game pid>` the game always spawns, and refuses the whole call — so the launcher read *"can only be terminated forcefully"* about the crash handler as *"there was nothing to ask"* about the game, and killed a world that had a window and a save to write. The ask no longer carries `/T`; the force still does, so a killed game takes its crash handler with it. A forced process skips the game's own shutdown, and save-on-quit is part of that shutdown. **`MULTIVERSE_SAVE_ON_QUIT` is not the setting at fault**: it is on, and the game never reaches the code that reads it. **But a window is not the only way in.** The mod polls a command file of its own — `MULTIVERSE_CMD_FILE`, one per world, `<data root>\cmd.txt` — and its `quit` verb runs the same `Application.Quit()` a window close would have, which is the shutdown save-on-quit happens in. `stop` asks that way FIRST, and falls back to the window only when nothing takes the request. **The mod reads that variable once, at plugin start**, so a world that was already running before the launcher set it has no consumer for the file and can still only be stopped the old way — the stop says so, in as many words, within a second. On Linux there is no such gap either way — `stop-multiverse.sh` sends `SIGTERM` and waits, which a headless game handles | **START THE WORLD AGAIN ONCE, and every stop after that is lossless** — including a headless one. A world started by this release's launcher, or by a `Start-Multiverse.ps1` this release generated, is told where its command file is and is asked to save and quit through it; a world started before that was never told, and nothing can be done to the one that is running now. The stop names that case: *nothing is reading `<data root>\cmd.txt`… Start this world again once and the next stop is lossless*. **Until you do**, the two older answers still hold: give a headless world a short save interval, so the most it can lose is that interval (`multiverse-launcher.exe profile set NAME --save-minutes 5`), or run it with a window for that session (`multiverse-launcher.exe start --no-headless`) and stop it normally. **The stop says which of the three happened**, so you never have to guess whether a save was written: *asked through the mod, and it saved and quit* (nothing lost, window or no window), *asked to close, and it closed*, *forced after 30s: it was asked to close and had not closed by then*, or *forced immediately: it has no window* — and only the last of those prints this entry's remedy. If you see it for a world you thought had a window, that world was started headless. The launcher's `stop --game-only` has the same limit; it is the game's shutdown that is skipped, not the sidecar's | **you** |
| `LOCAL-JOURNALTORN` | On sidecar start, in this install's `logs\sidecar.log` (`%LOCALAPPDATA%\BibitesMultiverse` on Windows, `${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse` on Linux): `sidecar: the journal was damaged and replay stopped early; custody history after the torn record is GONE`, carrying `discardedBytes` and the journal's path | The journal was torn — ordinarily by a full disk, sometimes by a hard kill. **Complete records behind the tear were thrown away.** The healthy reading is zero bytes on every start, and a healthy start logs nothing at all here | Free disk, then report the byte count. The number is evidence and it is not recoverable afterwards | **you** |
| `LOCAL-JOURNALGROWS` **(Windows)** | In this install's `logs\sidecar.log`, once every compaction interval (15 minutes by default): `sidecar: journal compaction failed`, carrying `err="rename ...\data\journal\journal.log.tmp ...\data\journal\journal.log: Access is denied."` — and `data\journal\journal.log` grows without bound. The two sidecars that found it, on 2026-08-17, held **718 MB** and **132 MB** for a live set of a few entries | **The compaction renamed its rewritten copy over a file its own process still held open, and Windows refuses that.** The timer, the rewrite and the fsync were all working; only the last step failed, it failed the same way every time, and so from the first attempt the journal never shrank again. Linux never saw it, because there a rename over an open file succeeds. **No custody is at risk**: every record is in the log and replays: what it costs is disk | **Fixed 2026-08-17** — the compaction closes its append handle across the rename and opens it again after. **On an older build, restart the sidecar and the journal shrinks**: the compaction at start-up runs before the append handle exists and has always worked, which is why the file was large rather than endless. The launcher's `stop` then `start` is enough | **you** |
| `LOCAL-DISK` | Writes fail; the sidecar reports journal errors; the machine fills | The genome cache, the journal and the logs grow, and **no rule in this system will ever shrink the durable ones**. A full disk here has previously left thousands of zero-byte scratch files behind, spending inodes at the moment inodes were what had run out | Free space and keep it free. On a packaged install everything this software writes is under one data root — `%LOCALAPPDATA%\BibitesMultiverse` on Windows, `${XDG_DATA_HOME:-$HOME/.local/share}/bibites-multiverse` on Linux — where `data/` is the journal and the genome cache and `logs/` the sidecar's log, plus your worlds, which the game keeps in its own place (`%USERPROFILE%\AppData\LocalLow\The Bibites`, or `${XDG_CONFIG_HOME:-$HOME/.config}/unity3d/The Bibites/The Bibites`) and whose count is `MULTIVERSE_SAVE_KEEP`. Size the disk against a record that only grows | **you** |
| `LOCAL-PARTIALSAVE` | A `.partial.zip` beside the world's save files at rest | A run died mid-save. The previous good save is untouched — the order is write, verify, rotate, prune, and a failure at any step leaves the live save alone | Safe to delete; the mod deletes it itself next time it saves | **nobody** |
| `LOCAL-TWOSIDECARS` | Two sidecar processes are running against one data directory. `--diagnose`'s `stale-process` check names both process ids | **The journal is a single-writer file**, and two processes appending to one custody log is how custody history is lost. It is the local twin of the disk failure above: nothing refuses, nothing logs an error, and the damage is only visible at the next replay | Stop one of them. On a packaged install the launcher's `stop` — or the stop script, `Stop-Multiverse.ps1` or `stop-multiverse.sh` — stops the one this install started; a second copy started by hand is the one to find | **you** |
| `LOCAL-STALEPID` | `<data-dir>/sidecar-process.json` names a process that is not running. `--diagnose` warns; nothing else notices | A sidecar was killed rather than stopped, and its process record outlived it. **Pid numbers are reused after a reboot**, so a record nobody removed is a record nobody should trust — and it is what makes a status query claim the thing is running when it is not | None: the next start overwrites it, and a clean stop removes it. It is worth knowing rather than fixing | **nobody** |

---

## 6. Refusals an operator causes on purpose

These are not faults. They are the escape hatches the map has always had, and a participant
should be able to recognise one rather than debug it.

**None of them is reachable from this wire.** Release, handover and eviction run over a
separate authenticated path that no peer and no subscriber can dial, and no frame a participant
sends invokes one. Each act takes **two calls**: the first returns a consequence report — the
slot, its position, its peer id, how long it has been dark, whose lanes change, which positions
become holes — and a single-use token bound to the map's current state; the second performs the
act and is refused if the map moved underneath the token. Every act leaves one durable audit
line. **A confirmation an operator cannot see is not a confirmation**, which is why the report
comes first (`contract-b-m4.md` §22, B28).

| Id | Symptom | Meaning | Remedy | Who acts |
|---|---|---|---|---|
| `OPS-EVICT` | `B-4005-evicted` | The operator closed this peer and is refusing it for a stated period. It is a **liveness** act, not a placement act: the reservation, the slot number and the position all survive untouched and the return is an ordinary reclaim. **Nothing distinguishes it on the wire from a draining relay**, by design — see `B-4005-evicted` | Contact the operator, once a routine restart has been ruled out | **operator** |
| `OPS-RELEASE` | Migrations addressed to a slot get a permanent `BMIG-SLOT_VACANT`, and the position appears as a hole on the map | The operator released a slot. The position rejoins the map as an ordinary hole the next newcomer fills before any axis grows, and **the address stays retired forever** | None. This is the operator's answer for a position that will never be filled again. The relay cannot enumerate the journals addressed to that slot — they are on other people's machines — so the consequence report says so and names where the answer is, which is the peer | **operator** |
| `OPS-HANDOVER` | A slot's identity changes; the new occupant starts with an empty journal. **The old identity's credential stops working at that moment** | The operator rebound a reservation — slot number **and** position — to a different identity. The map does not change shape and no lane moves. The act **mints a fresh credential for the new identity and drops the old one's**, and the new join string is printed once, in the reply to the confirming call: that is the only moment the new secret exists anywhere but its owner's machine. **It is also the only credential recovery path there is**, and the relay refuses to run one while the old world is still connected | None, and one thing to know: work in flight addressed to that slot **arrives at whoever is there now**, because routing is on the slot. An operator who does not want that outcome wants a release instead | **operator** |
| `OPS-DENYLIST` | A species name from your world renders as suppressed on the map's page or in its terminal view | The operator applied a render-time deny list at the archive's own boundary, so the page and the terminal view suppress the same strings. **Your world is untouched** — no eviction, no wire field, no cooperation asked of your machine, and nothing your sidecar or your game has to do | Contact the operator about the string. **What cannot be promised is removal from the record**: the map's record of what crossed — the species, the counts, the dates, the family links — is kept for the whole run and beyond, so M5 promises removal from the view and explicitly not from the record. The crossing lines behind those counts do age off the server on a stated schedule, and **that schedule is not a takedown either**: it ages lines by date and cannot pick out a world, a species or an organism | **operator** |

**Suppression is the weaker tool and it is reached for first.** It needs no peer's cooperation
and costs that peer nothing; an eviction takes a world off the map. The order is deliberate
(`contract-b-m4.md` §22, B28, B30).

---

## 7. Things that look like refusals and are not

Written down because each one has already cost somebody an evening somewhere.

- **Close `1000`.** A clean shutdown. A world unloaded or a game quit.
- **`A-4006` on your own restart.** The self-healing rule, working.
- **`BGEN-unknown_hash`.** A normal answer to a question about a genome nobody kept.
- **A closed north edge on a one-row map.** A map shape. It stays closed for the life of the
  map, by arithmetic: an axis of one has no other position to walk to.
- **A one-by-one map refusing nothing.** A lone first peer on a map that has not grown yet is
  the normal opening state of every map, and no export set is unusable there. `A-4007` fires
  only when the map can route something and this peer declared none of it.
- **An organism the exclusion policy kept at home.** It stays in its world, containment holds
  it, and the census still counts it. It is a policy, not a failure.
- **A parent with no blob in the record.** `parent_gone` is the ordinary case — the game drops a
  parentage once the parent is destroyed — and `blob_dropped_for_size` is a size ceiling doing
  its job on a crowded frame. Both are gaps, both are recorded as gaps, and neither refuses a
  migration or costs an organism.
- **A census marked truncated.** The world has more species than the wire carries; the array
  is a top slice, and the marker says so rather than pretending otherwise.
- **An absent value on the status page.** Absence means **unknown**, and a reader must not
  render it as zero. A slot that reports nothing is unknown, not empty.
- **A forwarded migration that sits unanswered for hours.** Nothing is stuck and nothing is
  retrying: the organism was handed over once, and the sender is waiting for an answer it will
  take no action without. It is not re-sent, and it does not come home. At `forwardTimeoutMs` —
  24 hours from the moment it was forwarded — the entry is closed as **lost**
  (`contract-b-m4.md` §9.3).
- **A `lost` count that is not zero.** It is what migration costs on this map, not a defect.
  Every world publishes its own count and the status page and `ringstat` both show it, because
  **a loss is a fact the operator reads rather than a silent repair**. A count that rises names
  a lane whose far end keeps disappearing mid-crossing; a count that rises on one world alone
  names that world's neighbour. The trade is stated in `contract-b-m4.md` §25, B37 and it was
  made in one direction only: **losing an organism is accepted, and duplicating one is not.**
- **A late `MIGRATION_ACK`, against an entry already written off.** Good news, and the sidecar
  logs it as such: the organism *did* arrive, exactly one of it exists, and its answer simply
  outran the deadline. What it names is a setting, not a fault — raise `--forward-timeout`
  (`MULTIVERSE_FORWARD_TIMEOUT`) above this map's slowest honest answer.

---

## 8. What to send when you ask for help

**The short version: send `multiverse-sidecar --diagnose --json`.** It carries all five of the
things below that a machine can know, it prints no secret at any verbosity, and the shape is
stable across releases so a report from an old build is still readable. What it cannot know is
what you were doing at the time.

A support conversation that starts with these five things skips a round trip:

1. **The taxonomy id**, if you found one. It is the whole of "what happened" in eight
   characters. `--diagnose` prints one against every failure and warning it reports.
2. **Four versions**: the game's, the mod's, the sidecar's, and the contract versions each
   wire reported. Two of them are on the map's own page, for every world at once.
3. **The close code and its reason string**, verbatim. No side parses a reason string; it is
   written for a person, and it is the one place the map explains itself.
4. **The log lines either side of it** — the sidecar's and the game's mod log. **On Linux, say
   whether more than one game instance has ever run out of that game folder**, because if one has,
   the mod log is not evidence: see `LOCAL-LOGSHRED`. `file BepInEx/LogOutput.log` answering
   `data` rather than text settles it in one command.
5. **Your world's identity** — the peer id and the slot. Both are public. **Your platform** is
   worth a word too: two of the entries in §1 and one in §5 exist on one platform only.

**Never send a credential or a token, in whole or in part.** No log in this system prints one,
at any level, and no diagnostic asks for one. A message that contains one is a message that
has to be treated as a compromise, which turns a support question into a slot handover.

---

## 9. The slots

The hosted map publishes each live slot's game build on its public live console. This closes
WP3's `B-4003c` documentation slot without adding a private operator lookup.

**WP7's own two are closed by its implementation arc.** §2.4 now names the two commands, states
that the sidecar must be stopped for both, and quotes the duplication warning the release prints
before it acts. And **`--diagnose` maps this whole document onto three exit codes, not one per
entry**: `0` when nothing failed, `1` when something did, `2` when the diagnostic itself could
not run. A code per taxonomy entry was the shape the slot imagined and it is the wrong one — the
id is already in the output, in the machine-readable form as a field, and an exit code that
carried it would be an unstable integer that changes whenever a table is reordered. The
[specification](sidecar-diagnose-spec.md) §1 fixes the codes and the JSON.

**WP6's two slots are closed.** §1 now names the release's setup, archives, and checksum chain,
quotes the checksum and unblock commands, adds the refusals the package itself invents
(`INS-EXECPOLICY`, `INS-JOINSTRING` and the two uninstall refusals), and carries
`INS-GAMEBUILD`'s text as the matrix words it. The dual-edition package also owns
`INS-RUNTIME`, which refuses to overwrite a changed managed game. §5 names the log files a packaged install
writes, quotes the mod's `configuration failed` line and the sidecar's torn-journal line, gives
the single-world form of the starvation tell, and names the save and disk settings as the player
meets them in the start script.

**WP6's Linux extension is landed with it.** §1 gains `INS-NOTEXECUTABLE` and `INS-LINUXDEPS`,
marks the two Windows-only entries as such, and re-states `INS-GAMEBUILD` on the matrix's new
(game version, platform) key. §5 gains **`LOCAL-LOGSHRED`**, measured on the rehearsal rather than
reasoned about: five instances out of one game directory, no rotated log at all, and 200 557 NUL
bytes in a 227 402-byte file that `file` reports as `data`. It is entered beside
`LOCAL-STARVATION` on purpose — the two are one act with opposite symptoms, and the pairing is the
thing a reader needs.

**WP2's and WP4's slots are closed** (0393698, dc9d01f, dfbd1dc). §3.1's and §3.2's refusal
texture, the operator-side view of an impersonation refusal, the published capacity table, the
admin path's participant-visible effects and the A49/A50 texture are now quoted from what those
packages ship rather than owed by them. **What a `4007` names, and what an evicted peer is
told, are answered above** — the second by a deliberate silence rather than by a new signal,
and that answer is pinned by a test so it cannot drift into one.

**This document closes when the exit test's error sweep finds no failure that is not in it,
with a remedy that worked** (`m5_considerations.md`, *Exit Test*, Part 6).
