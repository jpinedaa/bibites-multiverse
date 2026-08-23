<#
.SYNOPSIS
    Install The Bibites, the Multiverse mod, and its sidecar. Connect the game
    to the public map automatically.

.DESCRIPTION
    Runs on Windows PowerShell 5.1 and on PowerShell 7. It needs no compiler, no
    SDK, no runtime and nothing from a developer's toolchain, and it needs no
    administrator rights. It starts the game only when -StartAfterInstall is
    set. The graphical installer selects that option by default.

    Ahead of everything below there is a step 0 that writes nothing at all.
    Windows will not replace a program while it is running, so the installer
    first names every launcher, sidecar and game running out of the folders it is
    about to write into. If it finds one it says which, says how to stop it, and
    stops before it has changed anything (INS-BUSY, exit code 3). It never ends
    anything for you: a world ended rather than stopped loses everything it has
    simulated since its last save.

    What it does, in order:

      1. verifies every file in this folder against MANIFEST.sha256, then clears
         the mark of the web from the files it verified - and from nothing else;
      2. selects The Bibites: an existing Steam copy in the add-on edition, or
         the verified managed payload in the complete edition;
      3. checks your game build against support-matrix.json and STOPS if there is
         no entry for it, in the matrix's own words;
      4. installs BepInEx, if it is not already there;
      5. copies the plugin into BepInEx\plugins;
      6. gets a unique public-map identity, or splits a private-map join string,
         and stores the secret half in a file only you can read;
      7. imports a certificate authority ONLY if you gave it one with -CaFile,
         which a public map does not need;
      8. states the settings this install ships with, including the export
         default;
      9. installs the launcher this world runs from, writes the launcher's
         profile for it, writes Start-Multiverse.ps1 and Stop-Multiverse.ps1
         for console use, and writes the record the uninstall reads.

    IT WILL NEVER ASK YOU TO TURN A SECURITY CONTROL OFF. No part of this
    package prints an -ExecutionPolicy Bypass, an --insecure- flag, or an
    instruction to skip certificate verification. If any tool or page in this
    project asks you for one of those, that is the defect and reporting it is
    the right response.

    WHAT A BARE INSTALL DOES. Your world exports on ALL FOUR EDGES. Nothing
    configured means the whole perimeter, not silence. Step 8 states it again on
    your screen, with what each shipped setting costs you.

.PARAMETER JoinStringFile
    A file whose first non-empty line is the one-line join string your map's
    operator handed you:

        multiverse-join/1 wss://<relay-host>/contract-b/v4 <peerId>.<secret>

    Leave it out to connect to the public map automatically. If this package
    has no public-map.json, the installer asks for the join string at the
    keyboard, with the typing hidden. THERE IS DELIBERATELY NO -JoinString
    PARAMETER: a secret on a command line is in every process listing on this
    machine and in your shell history. The wire itself has the same rule - no
    flag anywhere takes a secret literally.

.PARAMETER RelayUrl
    Only needed if your operator gave you the two halves separately rather than
    as a one-line join string. It must be a wss:// address; this wire is always
    encrypted and there is no plain fallback.

.PARAMETER CaFile
    ONLY for a private or LAN map whose relay uses its own certificate
    authority. A public map's relay presents a certificate your machine already
    trusts, so leave this out and NOTHING is imported into any trust store.
    Given one, the installer prints what trusting it means, imports it into your
    own user store, and records the thumbprint so the uninstall can take it out
    again.

.PARAMETER GameDir
    Existing-game selection only: the game folder, if the installer does not
    find it. The portable-game selection refuses this parameter.

.PARAMETER RuntimeSelection
    Select the included portable game or an existing game. The default is the
    included game when the package contains one.

.PARAMETER StartAfterInstall
    Start the sidecar and game after a successful installation. The start
    script waits until the map grants this world a place before it starts the
    game.

.PARAMETER DataRoot
    Where this install keeps its journal, its credential, its logs and its
    record. A complete edition also keeps its versioned game runtime here.
    Defaults to %LOCALAPPDATA%\BibitesMultiverse.

.PARAMETER InstallRoot
    Where this install keeps its launcher, sidecar, icon, and uninstaller.
    The executable setup uses %LOCALAPPDATA%\Programs\Bibites Multiverse.

.PARAMETER World
    The name of the world this install runs. Defaults to Multiverse. The game
    seeds it on the first start.

.PARAMETER ExportEdges
    The edges your world exports through. THE DEFAULT IS ALL FOUR and the
    installer writes the value explicitly, so a future change to the mod's own
    default cannot silently move what your world does.

.PARAMETER ExcludeSpecies
    Species that never leave your world. Defaults to the game's starter species,
    'Basic bibite', which keeps founder stock off a shared map's lanes.

.PARAMETER NoMigrationExclusion
    Turn the exclusion policy off entirely. It takes its own switch on purpose:
    an empty -ExcludeSpecies is refused rather than silently disabling the
    policy, because that is a change a shared map notices and a census does not.

.PARAMETER ReplaceWorldIdentity
    Let the world in this data root become a DIFFERENT identity. Two situations
    need it, and they are the same act on disk:

      * a SLOT HANDOVER, which is the map's only credential recovery. It rebinds
        your slot and your position to a new identity with a freshly minted
        credential, and the operator sends you its join string. Nothing is
        stranded: your place moved with you.
      * a LOST SECRET. Nothing recovers one - the relay keeps a verifier and
        cannot print a secret again - so a new identity is the only way on, and
        it is a SECOND place on the map. The old world's slot stays reserved,
        with nobody in it, until its operator releases it.

    The installer cannot tell those apart, so it refuses both by default rather
    than strand a world quietly. It keeps the old name in
    <data root>\data\peer-id.previous and prints it, because that string is what
    the operator needs, and any secret it replaces is kept beside the new one.

.EXAMPLE
    .\Install-BibitesMultiverse.ps1
    Installs the included game and connects it to the public map.

.EXAMPLE
    .\Install-BibitesMultiverse.ps1 -JoinStringFile .\join.txt -World MyWorld

.EXAMPLE
    .\Install-BibitesMultiverse.ps1 -JoinStringFile .\join.txt -CaFile .\ca.crt
    A private or LAN map, whose relay signs its own certificate.
#>
[CmdletBinding()]
param(
    [string]$JoinStringFile = '',
    [string]$RelayUrl = '',
    [string]$CaFile = '',
    [string]$GameDir = '',
    [ValidateSet('auto', 'bundled', 'external')][string]$RuntimeSelection = 'auto',
    [string]$DataRoot = '',
    [string]$InstallRoot = '',
    [string]$World = 'Multiverse',
    [string]$ExportEdges = 'E,N,W,S',
    [string]$ExcludeSpecies = 'Basic bibite',
    [switch]$NoMigrationExclusion,
    [int]$SidecarPort = 8787,
    [double]$SaveMinutes = 10,
    [int]$SaveKeep = 6,
    [ValidateSet('on', 'off')][string]$SaveOnQuit = 'on',
    [switch]$StartAfterInstall,
    [switch]$SkipCaImport,
    [switch]$ReplaceWorldIdentity
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$Release      = '0.3.4'
$Here         = Split-Path -Parent $MyInvocation.MyCommand.Path
$ManifestName = 'MANIFEST.sha256'
$MatrixName   = 'support-matrix.json'
$PayloadDescriptorName = 'game-payload.json'
$PluginName   = 'BibitesMultiverse.dll'
$SidecarName  = 'multiverse-sidecar.exe'
$PluginGuid   = 'dev.multiverse.bibites'
$StartName    = 'Start-Multiverse.ps1'
$StopName     = 'Stop-Multiverse.ps1'
$RecordName   = 'install-record.json'
$PublicMapName = 'public-map.json'
$LauncherName = 'BibitesMultiverseLauncher.exe'
$ProfilesDirName = 'profiles'
$ProfileFormat = 'bibites-multiverse/launcher-profile/1'

# The speed a world starts at, written into $StartName beside the mod's other
# settings. It is a constant rather than a parameter, like the portal switches
# and unlike the save settings: it is not part of this world's identity, and the
# in-game speed slider already moves it for a session. The launcher writes the
# same value (go/internal/launcher/run.go).
$StartupTimeScale = '10'

# How long the start script waits for the game's mod to reach the sidecar before
# it warns. The game seeds a world on a first start, which is the slow case, so
# this is generous; it never fails the start (LOCAL-CONFIGRACE).
$ModWaitSeconds = 120

$discoveryScript = Join-Path $Here 'Find-BibitesGame.ps1'
if (-not (Test-Path -LiteralPath $discoveryScript -PathType Leaf)) {
    Write-Host "STOP [INS-CHECKSUM]: Find-BibitesGame.ps1 is missing." -ForegroundColor Red
    exit 1
}
. $discoveryScript

function Say  { param([string]$m) Write-Host "     $m" }
function Step { param([string]$m) Write-Host ""; Write-Host "==== $m" }
function Stop-Setup {
    param([string]$m, [string]$Id = '')
    Write-Host ""
    if ($Id) { Write-Host "STOP [$Id]: $m" -ForegroundColor Red }
    else     { Write-Host "STOP: $m" -ForegroundColor Red }
    Write-Host ""
    Write-Host "     Every refusal this project can hand you is listed, with its remedy and who"
    Write-Host "     has to apply it, in docs/error-taxonomy.md on the release page."
    exit 1
}

function Get-Sha256 {
    param([string]$Path)
    return (Get-FileHash -Path $Path -Algorithm SHA256).Hash.ToUpperInvariant()
}

# ------------------------------------------------- 0. nothing of this may be running

# WINDOWS WILL NOT REPLACE A PROGRAM WHILE IT IS RUNNING. The launcher, the
# console launcher, the sidecar and the mod are all files this installer
# overwrites, and Windows holds every one of them open for as long as something
# is running out of it. An install started over a live world used to get as far
# as step 9 and die there on a raw Copy-Item error naming a line number in this
# script - with steps 1 to 8 already done, so the mod inside the game had been
# replaced and this world's map identity had already been settled, and with the
# setup around it quitting before it wrote the shortcuts or the entry in
# Installed apps. Half an install is not what anybody asked for.
#
# So the question is asked FIRST, before this script writes anything at all, and
# the refusal names every process by name, by id and by path, and says how to
# stop each one. NOTHING IS EVER ENDED FOR YOU HERE: a world ended rather than
# stopped loses everything it has simulated since its last save, and only the
# person at the keyboard knows whether this second is the right one for that.

# A dedicated exit code, because this is the one refusal a caller can act on
# without reading a word of the log: something has to be closed, and nothing was
# changed. The graphical installer gives it its own dialog and a longer tail of
# this output (Install-BibitesMultiverse-Gui.ps1).
$ExitBusy = 3

# The files a running copy of the game holds open, relative to the game folder.
# They are what settles it when a process's own path cannot be read at all.
$GameLockProbe = @('The Bibites.exe', "BepInEx\plugins\$PluginName")

# The launcher ships under two names: the windowed program a shortcut opens, and
# the console program that takes `stop --all`. BOTH are asked about in every
# package, including one that carries only the first - a name that is not
# installed costs one query that finds nothing.
$ConsoleLauncherName = 'multiverse-launcher.exe'

function Test-FileLocked {
    # Ask the file itself, rather than asking about processes. A running Windows
    # process holds its own executable image - and every DLL it has loaded -
    # open with no write sharing, so an open for ReadWrite that is refused as a
    # sharing violation means something is running out of that file. An open
    # that succeeds means nothing is. A file that is not there cannot be locked.
    #
    # A refusal this account cannot tell apart from a lock - a folder it may not
    # write to at all - is reported as locked, because that is still the safe
    # answer, and it is the answer this check gave before it could tell.
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $false }
    $stream = $null
    try {
        $stream = [System.IO.File]::Open($Path,
                                         [System.IO.FileMode]::Open,
                                         [System.IO.FileAccess]::ReadWrite,
                                         [System.IO.FileShare]::None)
        return $false
    } catch [System.IO.IOException] {
        return $true
    } catch [System.UnauthorizedAccessException] {
        return $true
    } catch {
        return $true
    } finally {
        if ($stream) { $stream.Dispose() }
    }
}

function Get-FullPath {
    # A path this script can compare a process's image path against. It never
    # asks the path to exist: the folder an install is about to create is exactly
    # the folder nothing can be running out of yet.
    param([string]$Path)
    if (-not $Path) { return '' }
    $candidate = $Path
    if (-not [System.IO.Path]::IsPathRooted($candidate)) {
        $candidate = Join-Path (Get-Location).Path $candidate
    }
    try { return [System.IO.Path]::GetFullPath($candidate) } catch { return '' }
}

function Test-SamePath {
    param([string]$A, [string]$B)
    if (-not $A -or -not $B) { return $false }
    $left  = $A.TrimEnd('\', '/')
    $right = $B.TrimEnd('\', '/')
    # Windows file names are case-insensitive, so this comparison is too. Every
    # comparison of a secret or an identity in this script is the opposite, and
    # is written -ceq / -cne for exactly that reason.
    return ($left -eq $right)
}

function Read-JsonFile {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $null }
    try {
        return (Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json)
    } catch {
        return $null
    }
}

function Get-JsonField {
    param($Object, [string]$Name)
    if ($null -eq $Object) { return '' }
    if ($Object.PSObject.Properties.Match($Name).Count -eq 0) { return '' }
    return [string]$Object.$Name
}

function Get-ProcessImage {
    # Every running process of one name, as a name, an id and an image path,
    # because a message somebody can act on needs all three. Win32_Process
    # answers with the three together, and leaves the path EMPTY for a process
    # this account may not inspect - another user's session, or an elevated one -
    # rather than throwing, which is the answer this check wants. If WMI itself
    # cannot be reached, Get-Process answers the same question.
    param([string]$Name)
    $rows = New-Object System.Collections.ArrayList
    $viaCim = $null
    try {
        $viaCim = @(Get-CimInstance -ClassName Win32_Process -Filter "Name = '$Name'" -ErrorAction Stop)
    } catch {
        $viaCim = $null
    }
    if ($null -ne $viaCim) {
        foreach ($row in $viaCim) {
            $image = ''
            if ($row.ExecutablePath) { $image = [string]$row.ExecutablePath }
            [void]$rows.Add([pscustomobject]@{ name = $Name; id = [int]$row.ProcessId; image = $image })
        }
        return $rows
    }
    $bare = [System.IO.Path]::GetFileNameWithoutExtension($Name)
    foreach ($process in @(Get-Process -Name $bare -ErrorAction SilentlyContinue)) {
        $image = ''
        try { $image = [string]$process.Path } catch { $image = '' }
        [void]$rows.Add([pscustomobject]@{ name = $Name; id = $process.Id; image = $image })
    }
    return $rows
}

function Get-BusyProcess {
    # What is running out of ONE folder, asked by program name. The question is
    # always the folder this install writes into and never the process name on
    # its own: one machine may hold a second copy of the game, or a second
    # world's launcher, and neither of them is a reason to refuse an install that
    # will not touch it. A Steam library elsewhere is somebody else's business.
    #
    # A process whose path this account cannot read is NOT counted on that fact
    # alone. It used to be, and that refused an install into a folder this
    # installer had just unpacked, on any machine that happens to run a second
    # world elsewhere. What settles it instead is the folder itself: -LockProbe
    # names the files a running copy holds open, and none of them open for
    # writing means nothing is running out of here, whoever else is running
    # whatever else.
    param([string]$Name, [string]$Root, [string[]]$LockProbe = @())
    $hit = New-Object System.Collections.ArrayList
    $full = Get-FullPath $Root
    if (-not $full) { return $hit }
    # THE TRAILING SEPARATOR IS THE WHOLE OF IT. A bare prefix test makes
    # C:\Bibites Multiverse match C:\Bibites Multiverse Two\launcher.exe, which
    # is a different install and a refusal nobody could explain.
    $prefix = $full.TrimEnd('\', '/') + [System.IO.Path]::DirectorySeparatorChar
    $opaque = 0
    foreach ($row in @(Get-ProcessImage $Name)) {
        if (-not $row.image) { $opaque++; continue }
        if ($row.image.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
            [void]$hit.Add(("{0}   process {1}   {2}" -f $row.name, $row.id, $row.image))
        }
    }
    if ($opaque -gt 0 -and $hit.Count -eq 0) {
        foreach ($relative in $LockProbe) {
            $candidate = Join-Path $full $relative
            if (Test-FileLocked $candidate) {
                [void]$hit.Add(("{0}   a copy this account cannot inspect   {1} is open with no write sharing" -f `
                                $Name, $candidate))
            }
        }
    }
    return $hit
}

function Stop-Busy {
    # The one refusal in this installer that is about a running program and not
    # about a file, a hash or a world. It never ends anything; it says what to
    # end, in the order that works - the worlds first, because the launcher is
    # what stops them, and the launcher itself after that.
    param([string[]]$Running, [switch]$Late, [string]$Detail = '')
    Write-Host ""
    Write-Host "STOP [INS-BUSY]: something of this installation is running, and Windows does not" -ForegroundColor Red
    Write-Host "                 let anything replace a program while it runs." -ForegroundColor Red
    Write-Host ""
    Say "running out of the folders this setup writes into:"
    foreach ($entry in $Running) { Say "  $entry" }
    if ($Detail) {
        Write-Host ""
        Say "what Windows said: $Detail"
    }
    Write-Host ""
    Say "Close them in this order, then run this setup again:"
    Write-Host ""
    Say "  1. STOP EVERY WORLD. Choose 'Stop every world' in the Bibites Multiverse"
    Say "     window, or run whichever of these your install has, in its own folder:"
    Say "         .\$LauncherName stop --all"
    Say "         .\$ConsoleLauncherName stop --all"
    Say "     A world this install's own script started stops with .\$StopName"
    Say "     beside it. Every one of those is lossless - the world is written out"
    Say "     before it closes, which ending it in Task Manager is not."
    Say "  2. CLOSE THE BIBITES MULTIVERSE WINDOW itself. The launcher's own program"
    Say "     file is one of the files this setup replaces, so it cannot be open while"
    Say "     that happens."
    Say "  3. CLOSE THE BIBITES if it is still open. Windows holds the mod's own file"
    Say "     open for as long as the game runs."
    Write-Host ""
    Say "NOTHING IS ENDED FOR YOU HERE. A world ended rather than stopped loses"
    Say "everything it has simulated since its last save, and that is not a setup"
    Say "program's decision to make."
    Write-Host ""
    if ($Late) {
        # THIS IS REACHED FROM STEP 5 AND FROM STEP 9, so it does not name the
        # steps it is standing after: every step this run printed above is done
        # and every step it did not print is not, which is a truer answer than a
        # list that would be wrong at one of the two.
        Say "WHAT THIS INSTALL HAD ALREADY DONE is printed above, step by step: every step"
        Say "you can see finished, and the ones after it did not start. NONE OF IT IS SPENT."
        Say "This world's map identity is reused rather than asked for twice, so running this"
        Say "setup again once nothing is running finishes what is left and costs you no"
        Say "second place on the map."
    } else {
        Say "NOTHING WAS CHANGED. This is the first thing this setup does: it asks before"
        Say "it has even checked its own package, and long before it touches the game, the"
        Say "mod or your world's identity. This machine is exactly as it was."
    }
    Write-Host ""
    Write-Host "     Every refusal this project can hand you is listed, with its remedy and who"
    Write-Host "     has to apply it, in docs/error-taxonomy.md on the release page."
    exit $ExitBusy
}

function Copy-ProgramFile {
    # Step 9's copy of a program file, with the sharing violation named. Step 0
    # above asks what is running before anything at all is written, so a lock
    # here means something STARTED during the install - a shortcut clicked, a
    # world started - and a raw "The process cannot access the file ... because it
    # is being used by another process" against a line number in this script is
    # not an answer anybody can act on. Every other copy failure is re-thrown
    # exactly as it arrived.
    param([string]$Source, [string]$Destination)
    try {
        Copy-Item -LiteralPath $Source -Destination $Destination -Force
    } catch {
        $failure = $_
        $blocked = $false
        $exception = $failure.Exception
        while ($exception) {
            if ($exception -is [System.IO.IOException]) { $blocked = $true; break }
            $exception = $exception.InnerException
        }
        if (-not $blocked -and (Test-FileLocked $Destination)) { $blocked = $true }
        if ($blocked) {
            Stop-Busy -Running @(("{0} is held open, so Windows will not replace it" -f $Destination)) `
                      -Late -Detail $failure.Exception.Message
        }
        throw $failure
    }
}

function Resolve-RuntimeSelection {
    # One installer, two package editions, and ONE place that decides which game
    # this run installs against. Step 0 needs the answer to know which game
    # folder has to be free, and step 2 needs the same answer to install into it;
    # a second copy of this rule would be a way for those two to disagree, and a
    # step 0 that guessed 'bundled' where step 2 went external would refuse an
    # install over a game it was never going to touch.
    param([string]$Selection, [string]$Game, [bool]$HasPayload)
    if ($Selection -ne 'auto') { return $Selection }
    if ($Game) { return 'external' }
    if ($HasPayload) { return 'bundled' }
    return 'external'
}

function Test-ManagedRuntimePath {
    # THE ONE FOLDER THIS INSTALLER MAY DELETE WHOLE, and the answer is no for
    # every other path on the machine. Step 2 rebuilds a managed runtime that is
    # no longer the game payload, and a recursive delete is the sharp end of
    # this script: the path it is given has to be proven, not trusted, because
    # the value it comes from is read off a descriptor and joined to a data root
    # a command line names.
    #
    # PROVEN MEANS ALL OF THIS, and any one of them failing is a refusal:
    #
    #   * no `..` anywhere in the path as written, refused before normalisation
    #     can turn it back into a folder that looks legitimate
    #   * its parent is EXACTLY <data root>\runtimes - not a folder below it, not
    #     the data root, not a sibling whose name starts the same way
    #   * its own name is a 64-character hex SHA-256, which is how the installer
    #     names a runtime and nothing else in that folder is called
    #   * and, when the caller knows which payload it is installing, that name is
    #     that payload's hash and no other runtime's
    #
    # This is the launcher's removeWorldData rule in the other direction: that
    # one deletes the entries it owns and keeps everything else, and this one
    # deletes one folder it owns whole and refuses every path that is not it.
    param([string]$Path, [string]$DataRoot, [string]$Sha = '')
    if (-not $Path -or -not $DataRoot) { return $false }
    if ($Sha -and $Sha -notmatch '^[0-9A-Fa-f]{64}$') { return $false }
    foreach ($segment in ($Path -split '[\\/]+')) {
        if ($segment -eq '..') { return $false }
    }
    $full = Get-FullPath $Path
    $runtimes = Get-FullPath (Join-Path (Get-FullPath $DataRoot) 'runtimes')
    if (-not $full -or -not $runtimes) { return $false }
    $me = $full.TrimEnd('\', '/')
    if (-not $me) { return $false }
    $parent = ''
    try { $parent = [System.IO.Path]::GetDirectoryName($me) } catch { return $false }
    if (-not $parent) { return $false }
    # Windows file names are case-insensitive and so is this, deliberately:
    # RUNTIMES\ is the same folder as runtimes\ and has to be as safe.
    if ($parent.TrimEnd('\', '/') -ne $runtimes.TrimEnd('\', '/')) { return $false }
    $leaf = [System.IO.Path]::GetFileName($me)
    if ($leaf -notmatch '^[0-9A-Fa-f]{64}$') { return $false }
    if ($Sha -and $leaf -ne $Sha) { return $false }
    return $true
}

function Test-InstallerOwnedBepInEx {
    # WHOSE BepInEx IS THIS, in ONE place, because everything the uninstall does
    # with the mod framework turns on the answer and a second copy of the rule is
    # how the install and the uninstall would come to disagree about it.
    #
    #   BepInEx that is not there yet   ours: this run is about to unpack it
    #   in a game somebody else chose   NOT ours. It was here before this install
    #                                   and may be another mod's; the uninstall
    #                                   leaves every file of it alone
    #   in the managed runtime          OURS, always. <data root>\runtimes\<sha>
    #                                   is a folder this installer creates from
    #                                   its own payload and nothing else writes
    #                                   in, so a BepInEx in it is one an earlier
    #                                   run of this installer put there
    #
    #   that THIS install put there     OURS AGAIN, whichever folder it is in.
    #                                   This is the UPGRADE case and it is the
    #                                   same mistake as the managed one in the
    #                                   other edition: the second install over a
    #                                   Steam or itch game found the framework it
    #                                   had unpacked itself one run earlier,
    #                                   called it a stranger's, and wrote an empty
    #                                   file list - and the uninstall then left
    #                                   the whole framework, its log, its config
    #                                   and its cache in somebody's own game
    #                                   folder for ever. The previous install's
    #                                   own record is what tells the two apart,
    #                                   and it is evidence rather than a guess: it
    #                                   names the game folder it says this about.
    param([bool]$Present, [string]$RuntimeMode, [string]$GameDir, [string]$DataRoot,
          [bool]$PreviouslyOurs = $false)
    if (-not $Present) { return $true }
    if ($RuntimeMode -eq 'bundled' -and (Test-ManagedRuntimePath $GameDir $DataRoot)) { return $true }
    return $PreviouslyOurs
}

function Get-PreviousInstall {
    # WHAT IS ALREADY HERE, read once, so that a setup run over an existing
    # installation is an UPGRADE and not a fresh install that happens to land on
    # top of one.
    #
    # WHY THIS EXISTS AT ALL. The graphical setup takes no arguments: somebody who
    # downloads the newest release from the homepage double-clicks it, and every
    # value this installer has is its own default. Before this, that meant an
    # upgrade REWROTE the launcher's world with those defaults - a world renamed
    # to 'Eden' in the launcher went back to 'Multiverse', a port moved off 8787
    # went back to 8787, and the launcher then opened a save that was not the one
    # the participant had been playing. THE SAVE FILE WAS NEVER TOUCHED; what was
    # lost was the installation's memory of which one was theirs, which from where
    # they sit is the same thing.
    #
    # TWO SOURCES, AND THE LIVE ONE WINS. The launcher's own profile is what the
    # launcher is running right now, and is the file a participant's own edits go
    # into (`multiverse-launcher profile set`). The install record is what THIS
    # installer last wrote, which is older the moment anybody changes anything. So
    # the profile is read first and the record is the fallback.
    #
    # BOTH ARE CHECKED AGAINST THIS DATA ROOT, the same rule Find-WorldIdentity
    # applies to the identity below: a profile that describes a different world's
    # folder says nothing about this one.
    #
    # IT WRITES NOTHING AND IT NEVER REFUSES. A file that is missing, unreadable,
    # or not what it claims to be leaves Present false and the install proceeds
    # exactly as it did before this function existed.
    param([string]$DataRoot, [string]$ProgramRoot)

    $previous = [pscustomobject]@{
        Present     = $false
        Record      = $null
        Profile     = $null
        ProfilePath = ''
        Active      = ''
    }
    if (-not $DataRoot) { return $previous }

    $record = Read-JsonFile (Join-Path $DataRoot $RecordName)
    $recordRoot = Get-JsonField $record 'dataRoot'
    if ($record -and ((-not $recordRoot) -or (Test-SamePath $recordRoot $DataRoot))) {
        $previous.Record = $record
        $previous.Present = $true
    }

    if ($ProgramRoot) {
        $profilesDir = Join-Path $ProgramRoot $ProfilesDirName
        $profilePath = Join-Path $profilesDir 'default.json'
        # $profileData, not $profile: $profile is one of PowerShell's own
        # automatic variables and shadowing it inside a setup script is a way to
        # be surprised later.
        $profileData = Read-JsonFile $profilePath
        if ($profileData -and (Test-SamePath (Get-JsonField $profileData 'dataRoot') $DataRoot)) {
            $previous.Profile = $profileData
            $previous.ProfilePath = $profilePath
            $previous.Present = $true
        }
        # active.txt is honoured only when it names a profile that is still there.
        # A name left behind by a world somebody deleted would send the launcher
        # looking for a file this install is not about to write.
        $activePath = Join-Path $profilesDir 'active.txt'
        if (Test-Path -LiteralPath $activePath -PathType Leaf) {
            $active = @(Get-Content -LiteralPath $activePath -ErrorAction SilentlyContinue) |
                      Select-Object -First 1
            if ($active) { $active = $active.Trim() }
            if ($active -and (Test-Path -LiteralPath (Join-Path $profilesDir "$active.json") -PathType Leaf)) {
                $previous.Active = $active
            }
        }
    }
    return $previous
}

function Get-PreviousSetting {
    # ONE VALUE, FROM THE STRONGEST SOURCE THAT HAS IT. The order is the whole
    # rule of an upgrade:
    #
    #   1. what the participant named ON THIS RUN. A value on the command line is
    #      an instruction, and an instruction is never overridden by history.
    #   2. the launcher's profile - what this installation is running now.
    #   3. the previous install record - what this installer last set.
    #   4. nothing, and the caller keeps its own default.
    #
    # $Named is $PSBoundParameters.ContainsKey(...) at the call site, which is the
    # only thing that can tell "-World Multiverse" from a default that happens to
    # equal it.
    param($Previous, [bool]$Named, [string]$ProfileField, [string]$RecordField, $Current)
    if ($Named -or -not $Previous.Present) { return $Current }
    if ($Previous.Profile -and $ProfileField -and
        $Previous.Profile.PSObject.Properties.Match($ProfileField).Count -gt 0) {
        return $Previous.Profile.$ProfileField
    }
    if ($Previous.Record -and $RecordField) {
        $source = $Previous.Record
        foreach ($part in ($RecordField -split '\.')) {
            if ($null -eq $source -or $source.PSObject.Properties.Match($part).Count -eq 0) {
                return $Current
            }
            $source = $source.$part
        }
        return $source
    }
    return $Current
}

Step "0 of 9 - check that nothing of this installation is running"

# WHERE IT LOOKS, AND WHY THOSE FOLDERS AND NO OTHERS. Every folder below is one
# THIS run writes into:
#
#   the application folder   step 9 replaces the launcher, the console launcher
#                            and the sidecar in it - and only when -InstallRoot
#                            names one. An advanced ZIP without it leaves those
#                            programs exactly where it was unpacked and copies
#                            none of them, so nothing there is at risk and
#                            nothing there is asked about.
#   the game folder          step 5 replaces the mod inside it. For the included
#                            portable game that is the managed runtime under the
#                            data root, whose folder name is the payload's own
#                            assembly hash - known from the descriptor before a
#                            single file is unpacked.
$payloadDescriptorPath = Join-Path $Here $PayloadDescriptorName
$hasBundledPayload = Test-Path -LiteralPath $payloadDescriptorPath -PathType Leaf
$busy = New-Object System.Collections.ArrayList

$plannedProgramRoot = ''
if (-not [string]::IsNullOrWhiteSpace($InstallRoot)) { $plannedProgramRoot = Get-FullPath $InstallRoot }
if ($plannedProgramRoot) {
    Say "application folder: $plannedProgramRoot"
    foreach ($program in @($LauncherName, $ConsoleLauncherName, $SidecarName)) {
        foreach ($entry in @(Get-BusyProcess $program $plannedProgramRoot @($program))) {
            [void]$busy.Add($entry)
        }
    }
} else {
    Say "application folder: none - this run installs no program files"
}

$plannedGameRoot = ''
if ((Resolve-RuntimeSelection $RuntimeSelection $GameDir $hasBundledPayload) -eq 'bundled') {
    $plannedDataRoot = Get-FullPath $DataRoot
    if (-not $plannedDataRoot -and $env:LOCALAPPDATA) {
        $plannedDataRoot = Get-FullPath (Join-Path $env:LOCALAPPDATA 'BibitesMultiverse')
    }
    $plannedSha = ''
    try {
        $plannedPayload = Get-Content -LiteralPath $payloadDescriptorPath -Raw | ConvertFrom-Json
        $plannedSha = ([string]$plannedPayload.assemblySha256).ToUpperInvariant()
    } catch {
        # A descriptor this cannot read is step 2's refusal to make, in its own
        # words and against the hash it computes itself. Step 0 asks about the
        # folder it can name and stays quiet about the one it cannot.
        $plannedSha = ''
    }
    if ($plannedDataRoot -and $plannedSha -match '^[0-9A-F]{64}$') {
        $plannedGameRoot = Join-Path (Join-Path $plannedDataRoot 'runtimes') $plannedSha
    }
} elseif ($GameDir) {
    $plannedGameRoot = Get-FullPath $GameDir
} else {
    # The same search step 2 runs, and it only reads. A game it does not find is
    # step 2's INS-GAMEPATH to refuse with rather than this check's.
    try { $plannedGameRoot = Get-FullPath (Find-BibitesGameDirectory) } catch { $plannedGameRoot = '' }
}
if ($plannedGameRoot) {
    Say "game folder       : $plannedGameRoot"
    foreach ($entry in @(Get-BusyProcess 'The Bibites.exe' $plannedGameRoot $GameLockProbe)) {
        [void]$busy.Add($entry)
    }
}

if ($busy.Count -gt 0) { Stop-Busy -Running @($busy) }
Say "nothing of this installation is running, so nothing has to be closed first."

# IS THIS AN UPGRADE? Read now, before anything is written, because the answer
# changes what four later steps do: which world the launcher's profile keeps
# (step 9), which settings the scripts are generated with (step 8), whose the mod
# framework is (step 4), and which files an older release left here that this one
# no longer ships (step 9).
#
# THE CANDIDATE ROOTS, RESOLVED WITHOUT CREATING EITHER OF THEM. -DataRoot when
# it was given, otherwise the same default steps 2 and 5 apply; -InstallRoot when
# it was given, otherwise the folder this kit was unpacked into. Nothing here
# makes a directory: an install that is about to refuse must leave no trace, and
# reading is all this needs.
$previousInstall = Get-PreviousInstall `
    -DataRoot   $(if ($DataRoot) { Get-FullPath $DataRoot } else { Join-Path $env:LOCALAPPDATA 'BibitesMultiverse' }) `
    -ProgramRoot $(if ($InstallRoot) { Get-FullPath $InstallRoot } else { $Here })
if ($previousInstall.Present) {
    $previousRelease = Get-JsonField $previousInstall.Record 'release'
    if ($previousRelease) {
        Say "updating the Bibites Multiverse $previousRelease already installed here."
    } else {
        Say "updating the Bibites Multiverse already installed here."
    }
    Say "Your worlds, their journals, their logs and this world's identity all stay where"
    Say "they are, and every setting you did not name on this run is kept as you left it."
}

# ---------------------------------------------------------------- 1. the kit

Step "1 of 9 - check this package against its own manifest"

$manifestPath = Join-Path $Here $ManifestName
if (-not (Test-Path $manifestPath)) {
    Stop-Setup ("$ManifestName is missing, so nothing here can be checked. Unpack the release " +
                "archive again, whole.") 'INS-CHECKSUM'
}

$verified = New-Object System.Collections.ArrayList
foreach ($line in (Get-Content -Path $manifestPath)) {
    $text = $line.Trim()
    if (-not $text -or $text.StartsWith('#')) { continue }
    $m = [regex]::Match($text, '^([0-9A-Fa-f]{64})\s+\*?(.+)$')
    if (-not $m.Success) { Stop-Setup "$ManifestName has a line this installer cannot read: $text" 'INS-CHECKSUM' }
    $want = $m.Groups[1].Value.ToUpperInvariant()
    $rel  = $m.Groups[2].Value.Trim()
    $file = Join-Path $Here $rel
    if (-not (Test-Path $file)) {
        Stop-Setup ("$rel is named in $ManifestName and is not in this folder. The package is " +
                    "incomplete; unpack the release archive again, whole.") 'INS-CHECKSUM'
    }
    $got = Get-Sha256 $file
    if ($got -ne $want) {
        Write-Host ""
        Write-Host "$rel is not the published file." -ForegroundColor Red
        Say "expected SHA-256: $want"
        Say "this copy       : $got"
        Stop-Setup ("Delete the whole folder and the archive, download the archive again from the " +
                    "release page, and check the archive's own checksum against the page BEFORE " +
                    "unpacking it. If it fails twice, report it and do not run it.") 'INS-CHECKSUM'
    }
    [void]$verified.Add($file)
}
Say ("{0} files match {1}." -f $verified.Count, $ManifestName)

# The mark of the web, and why this is the honest place to clear it. Windows
# marks every file that came out of a downloaded archive. The mark is a real
# control and this installer does not switch it off wholesale: it clears the
# mark from EXACTLY the files it has just checked against the manifest, by name,
# and from nothing else in this folder or anywhere on this machine.
$unblocked = 0
foreach ($file in $verified) {
    try {
        if (Get-Item -Path $file -Stream 'Zone.Identifier' -ErrorAction SilentlyContinue) {
            Unblock-File -Path $file
            $unblocked++
        }
    } catch {
        Say "could not clear the mark of the web on $file - $($_.Exception.Message)"
    }
}
if ($unblocked -gt 0) {
    Say "cleared the mark of the web from $unblocked of them, by name, after checking each one."
} else {
    Say "none of them carries the mark of the web; nothing to clear."
}

# ---------------------------------------------------------------- 2. the game

Step "2 of 9 - select The Bibites runtime"

# One installer, two package editions. A complete package carries this
# descriptor and a manifest-covered game\ tree. An add-on package carries
# neither and binds to an existing Steam copy.
$runtimeMode  = 'external'
$runtimeRoot  = ''
$runtimeFiles = @()
$payloadDescriptorPath = Join-Path $Here $PayloadDescriptorName
$hasBundledPayload = Test-Path -LiteralPath $payloadDescriptorPath -PathType Leaf
$RuntimeSelection = Resolve-RuntimeSelection $RuntimeSelection $GameDir $hasBundledPayload
if ($RuntimeSelection -eq 'bundled' -and -not $hasBundledPayload) {
    Stop-Setup 'This package has no included portable game. Select an existing game.' 'INS-RUNTIME'
}

if ($RuntimeSelection -eq 'bundled') {
    if ($GameDir) {
        Stop-Setup 'The portable-game selection does not use -GameDir.' 'INS-RUNTIME'
    }

    $payload = Get-Content -LiteralPath $payloadDescriptorPath -Raw | ConvertFrom-Json
    foreach ($required in @('format', 'platform', 'gameVersion', 'assemblySha256', 'redistributionNoticeFile')) {
        if ($payload.PSObject.Properties.Match($required).Count -eq 0) {
            Stop-Setup "$PayloadDescriptorName is missing its '$required' field." 'INS-CHECKSUM'
        }
    }
    if ($payload.format -ne 'bibites-multiverse/game-payload/1') {
        Stop-Setup "$PayloadDescriptorName has an unsupported format." 'INS-CHECKSUM'
    }
    if ($payload.platform -ne 'Windows') {
        Stop-Setup "This complete package carries a game payload for $($payload.platform), not Windows." 'INS-GAMEBUILD'
    }
    $payloadSha = ([string]$payload.assemblySha256).ToUpperInvariant()
    if ($payloadSha -notmatch '^[0-9A-F]{64}$') {
        Stop-Setup "$PayloadDescriptorName has an invalid assemblySha256." 'INS-CHECKSUM'
    }

    $payloadGameDir = Join-Path $Here 'game'
    $payloadNotice = Join-Path $Here ([string]$payload.redistributionNoticeFile)
    $payloadExe = Join-Path $payloadGameDir 'The Bibites.exe'
    $payloadAssembly = Join-Path $payloadGameDir 'The Bibites_Data\Managed\BibitesAssembly.dll'
    if (-not (Test-Path -LiteralPath $payloadGameDir -PathType Container)) {
        Stop-Setup 'The complete package is missing its game\ directory.' 'INS-CHECKSUM'
    }
    if (-not $payload.redistributionNoticeFile -or -not (Test-Path -LiteralPath $payloadNotice -PathType Leaf)) {
        Stop-Setup "The complete package is missing the redistribution notice named by $PayloadDescriptorName." 'INS-CHECKSUM'
    }
    if (-not (Test-Path -LiteralPath $payloadExe -PathType Leaf)) {
        Stop-Setup "The complete package has no 'The Bibites.exe' in game\." 'INS-CHECKSUM'
    }
    if (-not (Test-Path -LiteralPath $payloadAssembly -PathType Leaf)) {
        Stop-Setup 'The complete package is missing BibitesAssembly.dll.' 'INS-CHECKSUM'
    }
    if ((Get-Sha256 $payloadAssembly) -ne $payloadSha) {
        Stop-Setup "$PayloadDescriptorName does not describe the game assembly in this package." 'INS-CHECKSUM'
    }
    $links = @(Get-ChildItem -LiteralPath $payloadGameDir -Force -Recurse |
               Where-Object { ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0 })
    if ($links.Count -gt 0) {
        Stop-Setup 'The complete package contains a reparse point. Game payloads must contain regular files and directories only.' 'INS-CHECKSUM'
    }

    # Refuse an unsupported payload before copying anything. Step 3 repeats the
    # normal matrix gate against the installed runtime.
    $payloadMatrix = Get-Content -LiteralPath (Join-Path $Here $MatrixName) -Raw | ConvertFrom-Json
    $payloadEntry = @($payloadMatrix.entries | Where-Object {
        $_.platform -eq 'Windows' -and $_.assemblySha256.ToUpperInvariant() -eq $payloadSha
    }) | Select-Object -First 1
    if (-not $payloadEntry) {
        Stop-Setup "The game payload in this complete package is not in this release's support matrix. Nothing was installed." 'INS-GAMEBUILD'
    }

    if (-not $DataRoot) { $DataRoot = Join-Path $env:LOCALAPPDATA 'BibitesMultiverse' }
    New-Item -ItemType Directory -Force -Path $DataRoot | Out-Null
    $DataRoot = (Resolve-Path $DataRoot).Path
    $runtimeMode = 'bundled'
    $runtimeRoot = Join-Path (Join-Path $DataRoot 'runtimes') $payloadSha

    $payloadFiles = @(Get-ChildItem -LiteralPath $payloadGameDir -File -Force -Recurse | ForEach-Object {
        $relative = $_.FullName.Substring($payloadGameDir.Length) -replace '^[\\/]+', ''
        [pscustomobject]@{
            relative = $relative
            source   = $_.FullName
            sha256   = (Get-Sha256 $_.FullName)
        }
    })
    if ($payloadFiles.Count -eq 0) {
        Stop-Setup "The complete package's game\ directory contains no files." 'INS-CHECKSUM'
    }

    # A MANAGED RUNTIME IS A CACHE OF THIS PACKAGE'S OWN PAYLOAD. NOTHING IN IT
    # IS YOURS. <data root>\runtimes\<assembly sha256> holds a copy of the game
    # this package ships, named after the hash of the game assembly inside it,
    # and this installer is the only thing that writes there: your worlds are the
    # game's own saves under %USERPROFILE%\AppData\LocalLow, and your journal,
    # your logs and this world's credential are in the data root BESIDE it, not
    # in it. The launcher never writes in it either (docs/error-taxonomy.md §1).
    #
    # SO AN INCOMPLETE ONE IS REBUILT RATHER THAN REFUSED. Install, uninstall,
    # install again used to end here, in a refusal with no way past it but
    # deleting a folder by hand: the uninstall takes the files it recorded and
    # leaves what the game and BepInEx wrote after the install, so the folder
    # survives with no game left in it - and this step then found changelog.txt
    # missing and said "It was not overwritten." The folder is this package's,
    # what is left in it is not a game, and the payload to put back is right
    # here, verified against the manifest by step 1. It is rebuilt, and every
    # file removed to do it is counted on screen first.
    #
    # WHAT IS STILL REFUSED, and why the two cases are not the same: a runtime
    # that is COMPLETE and DIFFERENT. Every payload file is there and one of them
    # is not the file this package ships - a game copy somebody changed on
    # purpose, which still runs. That is not rubble and this installer does not
    # overwrite it.
    $stageRuntime = $true
    if (Test-Path -LiteralPath $runtimeRoot -PathType Container) {
        $missingFiles = New-Object System.Collections.ArrayList
        $changedFiles = New-Object System.Collections.ArrayList
        foreach ($file in $payloadFiles) {
            $destination = Join-Path $runtimeRoot $file.relative
            if (-not (Test-Path -LiteralPath $destination -PathType Leaf)) {
                [void]$missingFiles.Add($file.relative)
            } elseif ((Get-Sha256 $destination) -ne $file.sha256) {
                [void]$changedFiles.Add($file.relative)
            }
        }
        if ($missingFiles.Count -eq 0 -and $changedFiles.Count -eq 0) {
            Say 'complete edition: reusing the verified managed game runtime'
            $stageRuntime = $false
        } elseif ($missingFiles.Count -eq 0) {
            Stop-Setup ("The managed runtime at $runtimeRoot was changed ($($changedFiles[0]) differs). " +
                        "It was not overwritten.") 'INS-RUNTIME'
        } else {
            $leftBehind = @(Get-ChildItem -LiteralPath $runtimeRoot -File -Force -Recurse -ErrorAction SilentlyContinue)
            Say ("the managed game runtime in {0} is not the game any more:" -f $runtimeRoot)
            Say ("  {0} of {1} payload files are gone (the first is {2}), {3} of them differ," -f
                 $missingFiles.Count, $payloadFiles.Count, $missingFiles[0], $changedFiles.Count)
            Say ("  and {0} file(s) are left in it - what the game and BepInEx wrote while it ran." -f $leftBehind.Count)
            # NOTHING IS DELETED AT A PATH THIS CANNOT PROVE. The whole folder
            # goes, so the path has to be exactly the one this step owns - and
            # the reassurance below is only true of a path that passed, so it is
            # said after the delete rather than over one that is refused.
            if (-not (Test-ManagedRuntimePath $runtimeRoot $DataRoot $payloadSha)) {
                Stop-Setup ("The managed runtime at $runtimeRoot is incomplete, and this installer will " +
                            "not remove it: that path is not <data root>\runtimes\<payload sha256>, which " +
                            "is the only folder this step owns. Nothing was changed.") 'INS-RUNTIME'
            }
            try {
                Remove-Item -LiteralPath $runtimeRoot -Recurse -Force
            } catch {
                Stop-Setup ("The incomplete managed runtime at $runtimeRoot could not be removed: " +
                            "$($_.Exception.Message)") 'INS-RUNTIME'
            }
            Say "That folder was this package's own copy of the game and held nothing of yours:"
            Say "your worlds, this world's journal, its logs and its credential are all outside it."
            Say 'complete edition: removed the incomplete managed runtime whole; the payload is staged again below'
        }
    }
    if ($stageRuntime) {
        $runtimeParent = Split-Path -Parent $runtimeRoot
        New-Item -ItemType Directory -Force -Path $runtimeParent | Out-Null
        $runtimeTemp = Join-Path $runtimeParent ('.installing-' + [guid]::NewGuid().ToString('N'))
        New-Item -ItemType Directory -Path $runtimeTemp | Out-Null
        try {
            Get-ChildItem -LiteralPath $payloadGameDir -Force |
                Copy-Item -Destination $runtimeTemp -Recurse -Force
            $copiedAssembly = Join-Path $runtimeTemp 'The Bibites_Data\Managed\BibitesAssembly.dll'
            if ((Get-Sha256 $copiedAssembly) -ne $payloadSha) { throw 'the copied game assembly hash does not match' }
            Move-Item -LiteralPath $runtimeTemp -Destination $runtimeRoot
        } catch {
            Remove-Item -LiteralPath $runtimeTemp -Recurse -Force -ErrorAction SilentlyContinue
            Stop-Setup "The managed runtime copy failed verification: $($_.Exception.Message)" 'INS-RUNTIME'
        }
        Say 'complete edition: installed the verified game payload into a managed runtime'
    }
    foreach ($file in $payloadFiles) {
        $runtimeFiles += [pscustomobject]@{
            path   = (Join-Path $runtimeRoot $file.relative)
            sha256 = $file.sha256
        }
    }
    Say "game redistribution notice: $payloadNotice"
    $GameDir = $runtimeRoot
} else {
    if ($hasBundledPayload) {
        Say 'existing-game selection: the included portable game will not be copied'
    } else {
        Say 'add-on edition: binding to an existing game installation'
    }
}

if (-not $GameDir) { $GameDir = Find-BibitesGameDirectory }
if (-not $GameDir) {
    Stop-Setup ("Game not found. Select the folder that contains The Bibites.exe, or run this " +
                "script again with -GameDir 'D:\path\to\The Bibites'.") 'INS-GAMEPATH'
}
if (-not (Test-Path (Join-Path $GameDir 'The Bibites.exe'))) {
    Stop-Setup "There is no 'The Bibites.exe' in $GameDir." 'INS-GAMEPATH'
}
$GameDir = (Resolve-Path $GameDir).Path
Say "game directory: $GameDir"

$runningHere = @(Get-BusyProcess 'The Bibites.exe' $GameDir $GameLockProbe)
if ($runningHere.Count -gt 0) {
    # Step 0 asked this before anything at all was written, of the folder it
    # worked out this run would use. It is asked again here because THIS is that
    # folder resolved: a search that found the game, a -GameDir given as a
    # relative path, or the managed runtime step 2 has just unpacked. Nothing has
    # been written into the game or the data root yet either way.
    Stop-Busy -Running @($runningHere)
}

# ---------------------------------------------------------------- 3. the matrix

Step "3 of 9 - check this game build against the support matrix"

$matrixPath = Join-Path $Here $MatrixName
if (-not (Test-Path $matrixPath)) { Stop-Setup "The package is incomplete: $MatrixName is missing." 'INS-CHECKSUM' }
$matrix = Get-Content -Path $matrixPath -Raw | ConvertFrom-Json

$assembly = Join-Path $GameDir 'The Bibites_Data\Managed\BibitesAssembly.dll'
if (-not (Test-Path $assembly)) { Stop-Setup "The game assembly is missing: $assembly" }
$assemblySha = Get-Sha256 $assembly

# A matrix row is keyed on (game version, PLATFORM): one game version has one
# file per platform and they do not share a hash. This installer is the Windows
# one, so it looks up the Windows rows and nothing else - a Linux hash here is
# INS-GAMEBUILD rather than a confusing match.
$entry = $null
foreach ($candidate in @($matrix.entries)) {
    if ($candidate.platform -ne 'Windows') { continue }
    if ($candidate.assemblySha256.ToUpperInvariant() -eq $assemblySha) { $entry = $candidate; break }
}

if (-not $entry) {
    Write-Host ""
    Write-Host "The game on this machine is not a build this release supports." -ForegroundColor Red
    Write-Host ""
    # The matrix's own words, quoted from support-matrix.json rather than
    # written here, so the page a reader looks the refusal up on and the tool
    # that refuses cannot drift apart.
    foreach ($chunk in ($matrix.refusal -split '(?<=\.) ')) { Say $chunk }
    Write-Host ""
    Say "this machine's platform           : Windows"
    Say "this machine's BibitesAssembly.dll: $assemblySha"
    Say "the builds this release supports:"
    foreach ($candidate in @($matrix.entries)) {
        Say ("  game {0}  {1}/{2} ({3})  mod {4}, sidecar {5}" -f `
             $candidate.gameVersion, $candidate.platform, $candidate.store, `
             $candidate.storeBuild, $candidate.mod, $candidate.sidecar)
        Say ("      SHA-256 {0}" -f $candidate.assemblySha256)
    }
    Write-Host ""
    Say "A row is a game build AND a platform. If one of the rows above matches your hash on"
    Say "another platform, that is the release archive to download rather than this one."
    Say "The full matrix, and what a map with two game builds on it does, is in"
    Say "docs/support-matrix.md on the release page."
    Stop-Setup "the game build is not in the support matrix; NOTHING was installed." 'INS-GAMEBUILD'
}

Say ("game version {0} on {1}/{2} ({3}) - supported by this release" -f `
     $entry.gameVersion, $entry.platform, $entry.store, $entry.storeBuild)
Say ("this release: mod {0}, sidecar {1}, wire {2} and {3}" -f `
     $entry.mod, $entry.sidecar, $entry.contractB, $entry.contractA)
Say ("tested against: {0}" -f $entry.tested)

# ---------------------------------------------------------------- 4. BepInEx

Step ("4 of 9 - BepInEx {0}" -f $entry.bepInEx)

$bepInExZipName = "BepInEx_$($entry.bepInExFlavour)_$($entry.bepInEx).zip"
$bepInExCore    = Join-Path $GameDir 'BepInEx\core\BepInEx.dll'
$bepInExOurs    = $false
$bepInExHeld    = Test-Path $bepInExCore
$bepInExPaths   = New-Object System.Collections.ArrayList

# WHOSE BepInEx IS THIS? Test-InstallerOwnedBepInEx has the rule; this is what
# answering it wrong cost. A SECOND install over the same managed runtime found
# BepInEx already there, called it somebody else's, and recorded
# bepInEx.installedByThisInstaller=false with an empty file list. The uninstall
# then took its "it was here before us, leave it whole" branch for the framework
# - and for the log, the configuration and the cache that go with it - while
# removing the game payload it HAD recorded. What was left in
# <data root>\runtimes\<sha> was 27 files of BepInEx with no game around them
# (2.37 MB: no changelog.txt, no The Bibites.exe, no _Data), and the install
# after that found the folder incomplete and refused to overwrite it. The
# framework in the managed runtime was never anybody else's: this installer put
# it there itself, one run earlier.
#
# THE SAME MISTAKE HAD A SECOND HALF, and it is the one an upgrade over a Steam
# or itch game hits: this installer unpacked the framework into somebody's own
# game folder on install one, and on install two it saw BepInEx there, called it
# a stranger's, and wrote the empty list again - so the uninstall left the whole
# framework, its log, its config and its cache behind in a folder it had promised
# to put back exactly as it found it. Whose it was is not a guess: the previous
# install's own record says so, and it names the game folder it says it about.
$bepInExWasOurs = $false
$bepInExPreviouslyRecorded = @{}
if ($previousInstall.Record -and
    (Test-SamePath (Get-JsonField $previousInstall.Record 'gameDir') $GameDir) -and
    $previousInstall.Record.PSObject.Properties.Match('bepInEx').Count -gt 0 -and
    $previousInstall.Record.bepInEx) {
    $previousBepInEx = $previousInstall.Record.bepInEx
    # ConvertFrom-Json turns a JSON true into $true, and Get-JsonField hands it
    # back as the string 'True'.
    $bepInExWasOurs = ((Get-JsonField $previousBepInEx 'installedByThisInstaller') -eq 'True')
    if ($bepInExWasOurs -and $previousBepInEx.PSObject.Properties.Match('files').Count -gt 0) {
        foreach ($previousFile in @($previousBepInEx.files)) {
            $previousRel = Get-JsonField $previousFile 'path'
            if ($previousRel) { $bepInExPreviouslyRecorded[$previousRel.ToUpperInvariant()] = $true }
        }
    }
}
$bepInExOwned   = Test-InstallerOwnedBepInEx -Present $bepInExHeld -RuntimeMode $runtimeMode `
                                             -GameDir $GameDir -DataRoot $DataRoot `
                                             -PreviouslyOurs $bepInExWasOurs
# Ours AND inside this package's own game copy: then a file of the archive that
# is already on disk is one of this installer's own, and is recorded like the
# rest rather than being left for nobody to remove.
$bepInExManaged = $bepInExOwned -and ($runtimeMode -eq 'bundled')

if (-not $bepInExOwned) {
    Say "BepInEx is already installed here; left exactly as it is."
    Say "The uninstall will not touch it either - it removes only what it put there."
} else {
    $zip = Join-Path $Here $bepInExZipName
    if (-not (Test-Path $zip)) { Stop-Setup "The package is incomplete: $bepInExZipName is missing." 'INS-CHECKSUM' }

    # The game's own files are the runtime record's, never this list's. BepInEx's
    # archive lays files in the game's own root beside the game's, and one of
    # them can have a name the game also uses; a game file recorded here would
    # be claimed by two halves of one record.
    $payloadOwns = @{}
    if ($bepInExManaged) {
        foreach ($file in $payloadFiles) { $payloadOwns[$file.relative.ToUpperInvariant()] = $true }
    }

    # Every path the archive holds, read BEFORE anything is unpacked, so the
    # uninstall removes those files and nothing that was already here - except in
    # this package's own managed game copy, where a file of the archive that is
    # already on disk is one an earlier run of this installer put there, and is
    # recorded exactly like the rest.
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [System.IO.Compression.ZipFile]::OpenRead($zip)
    try {
        foreach ($zipEntry in $archive.Entries) {
            if (-not $zipEntry.Name) { continue }
            $rel = $zipEntry.FullName.Replace('/', '\')
            $relKey = $rel.ToUpperInvariant()
            if (Test-Path (Join-Path $GameDir $rel)) {
                # TWO WAYS A FILE ALREADY ON DISK IS THIS INSTALL'S. It is in this
                # package's own managed game copy, where nothing else writes; or
                # the previous install's record names it, which is this
                # installer's own signed statement that it put that file there -
                # and is the only evidence available in a game folder somebody
                # else chose. A GAME FILE IS NEITHER, whatever its name: the
                # runtime record owns those, and one path claimed by both halves
                # of one record is a file the uninstall would count twice.
                $mine = (-not $payloadOwns.ContainsKey($relKey)) -and
                        ($bepInExManaged -or $bepInExPreviouslyRecorded.ContainsKey($relKey))
                if (-not $mine) {
                    Say "left alone (already here): $rel"
                    continue
                }
            }
            [void]$bepInExPaths.Add($rel)
        }
    } finally {
        $archive.Dispose()
    }

    if ($bepInExHeld) {
        # NOTHING IS UNPACKED OVER IT. The framework in the managed game copy is
        # already the one this package ships - an earlier run of this installer
        # unpacked it there, and nothing else writes in that folder. What this
        # run changes is the RECORD: these files are named as this install's, so
        # the uninstall takes them with the game copy instead of leaving a folder
        # holding BepInEx and no game.
        if ($bepInExManaged) {
            Say ("BepInEx {0} is already in this package's own managed game copy ({1} files)." -f `
                 $entry.bepInEx, $bepInExPaths.Count)
            Say "Only this installer writes in that folder, so it is this install's to remove:"
            Say "the uninstall takes the framework with the game copy rather than leaving it behind."
        } else {
            Say ("BepInEx {0} is already here and the install before this one put it there ({1} files)." -f `
                 $entry.bepInEx, $bepInExPaths.Count)
            Say "It stays this install's to remove, so the uninstall still takes the framework out of"
            Say "your own game folder instead of leaving it there for ever."
        }
    } else {
        Expand-Archive -Path $zip -DestinationPath $GameDir -Force
        if (-not (Test-Path $bepInExCore)) { Stop-Setup "BepInEx did not unpack into $GameDir." }
        Say ("BepInEx {0} installed into the game directory ({1} files)." -f $entry.bepInEx, $bepInExPaths.Count)
        Say "The first game start after this is slower: BepInEx writes its configuration then."
    }
    $bepInExOurs = $true
}

# ---------------------------------------------------------------- 5. the plugin

Step "5 of 9 - the multiverse plugin"

$pluginSrc = Join-Path $Here $PluginName
if (-not (Test-Path $pluginSrc)) { Stop-Setup "The package is incomplete: $PluginName is missing." 'INS-CHECKSUM' }
$sidecarSrc = Join-Path $Here $SidecarName
if (-not (Test-Path $sidecarSrc)) { Stop-Setup "The package is incomplete: $SidecarName is missing." 'INS-CHECKSUM' }

$pluginDir  = Join-Path $GameDir 'BepInEx\plugins'
$pluginDst  = Join-Path $pluginDir $PluginName
$pluginHeld = Test-Path $pluginDst
New-Item -ItemType Directory -Force -Path $pluginDir | Out-Null
# THROUGH THE LOCK-AWARE COPY, like step 9's program files. Step 0 asks what is
# running before anything is written, so a lock here means a world was started
# DURING the install - and the mod is the file the running game holds open. A raw
# "The process cannot access the file ... because it is being used by another
# process" against a line number in this script is not an answer anybody can act
# on; Stop-Busy's is.
Copy-ProgramFile $pluginSrc $pluginDst
$pluginSha = Get-Sha256 $pluginDst
if ($pluginHeld) { Say "$PluginName replaced in $pluginDir (an earlier install was here)" }
else             { Say "$PluginName -> $pluginDir" }
Say ("mod version {0}, SHA-256 {1}" -f $entry.mod, $pluginSha.Substring(0, 16))

if (-not $DataRoot) { $DataRoot = Join-Path $env:LOCALAPPDATA 'BibitesMultiverse' }
New-Item -ItemType Directory -Force -Path $DataRoot | Out-Null
$DataRoot = (Resolve-Path $DataRoot).Path
if ($DataRoot.Contains("'")) {
    # The generated start and stop scripts carry this path inside a single-quoted
    # PowerShell string, and step 6 reads that line back to recover a world's
    # identity. A quote in the path breaks both. Refuse rather than write a script
    # that cannot parse.
    Stop-Setup ("The data directory's path contains a single quote, which the generated start " +
                "script cannot carry safely: $DataRoot. Pass -DataRoot with a path without one.")
}
$dataDir  = Join-Path $DataRoot 'data'
$logDir   = Join-Path $DataRoot 'logs'
New-Item -ItemType Directory -Force -Path $dataDir, $logDir | Out-Null
Say "this install's own files live in $DataRoot"

# ---------------------------------------------------------------- 6. the map identity

Step "6 of 9 - connect this installation to a map"

function Read-JoinString {
    param([string]$File)
    if ($File) {
        if (-not (Test-Path $File)) { Stop-Setup "No join string file at $File." 'INS-JOINSTRING' }
        foreach ($line in (Get-Content -Path $File)) {
            $candidate = $line.Trim()
            if ($candidate -and -not $candidate.StartsWith('#')) { return $candidate }
        }
        Stop-Setup "$File holds no join string. Its first non-empty line must be the one your operator sent." 'INS-JOINSTRING'
    }
    Write-Host ""
    Say "Paste the join string your map's operator handed you. It looks like this:"
    Say "    multiverse-join/1 wss://<relay-host>/contract-b/v4 <your-world>.<secret>"
    Say "The typing is hidden, because the second half of it is the whole of your"
    Say "world's identity on that map. Nothing echoes it and no log ever prints it."
    Write-Host ""
    $secure = Read-Host -Prompt '     join string' -AsSecureString
    $bstr = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        return [System.Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr).Trim()
    } finally {
        [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    }
}

$credential = ''
$pendingEnrollmentPath = ''
$usedAutomaticEnrollment = $false
$adoptedIdentity = $false
$credentialPath = Join-Path $DataRoot 'peer-secret.txt'

function Protect-UserFile {
    param([string]$Path, [string]$Description, [switch]$Required)
    $currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
    try {
        $acl = New-Object System.Security.AccessControl.FileSecurity
        $acl.SetAccessRuleProtection($true, $false)
        $acl.AddAccessRule((New-Object System.Security.AccessControl.FileSystemAccessRule($currentUser, 'FullControl', 'Allow')))
        (Get-Item -LiteralPath $Path).SetAccessControl($acl)
        Say "$Description is readable by $currentUser only"
        return $true
    } catch {
        if ($Required) {
            Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
            Stop-Setup ("The installer could not protect its pending map credential. It did not " +
                        "contact the map. $($_.Exception.Message)") 'INS-ENROLL'
        }
        Say "$Description is in $Path"
        Say "WARNING: the permissions could not be tightened: $($_.Exception.Message)"
        Say "The file is inside your own profile, which other accounts cannot read by default."
        return $false
    }
}

# ONE IDENTITY PER WORLD, AND THE IDENTITY BELONGS TO THE DATA ROOT. Installing
# again over a data root that already holds a world is an upgrade or a repair,
# never a second world: it keeps that world's identity, its relay and its secret
# file untouched. Only an empty data root asks the map for an identity.
#
# The identity is written in more than one place, because no single one of them
# survives everything. These are searched strongest binding first:
#
#   1. <data root>\install-record.json         the install that owns this folder,
#                                              and only when it names this folder
#   2. <data root>\enrollment-pending.json     ONLY when its secret is the secret
#                                              on disk, which binds the two
#   3. <program root>\profiles\*.json          the launcher's own copy, matched
#                                              on this same data root
#   4. <program root>\Start-Multiverse.ps1     what 0.2.0-0.2.3 left behind
#   5. <data root>\data\peer-id (+ relay-url)  what the sidecar persists and what
#                                              this installer writes, and the last
#                                              thing an uninstall keeps
#   6. <data root>\logs\sidecar.log*          THE WORLD'S OWN VOICE. Every line
#                                              the sidecar writes carries peer=,
#                                              because the identity is an
#                                              attribute of its logger, and its
#                                              startup line carries relay= too
#
# THE PROGRAM ROOTS 3 and 4 are searched in are bounded and named: -InstallRoot,
# this kit's own folder, %LOCALAPPDATA%\Programs\Bibites Multiverse, the data
# root itself, and the data root's parent - because a ZIP kit is often unpacked
# beside the data it writes. There is deliberately NO wider search: an installer
# that walks a disk looking for a world is an installer nobody can predict.
#
# PROVEN vs CLAIMED. 1 and 2 are proven: the record is written by the same run
# that writes the credential and names this data root, and the pending record is
# accepted only when it carries the very secret on disk. 3, 4, 5 and 6 are claims
# - ordinary text files, and 5 is the one this installer's own refusal asks a
# participant to write by hand. A claim is enough to ADOPT a world, which changes
# nothing on disk, and never enough to OVERWRITE a secret.

# Test-SamePath, Read-JsonFile and Get-JsonField used to be defined here. They
# are up beside Get-FullPath now, because STEP 4 READS THE PREVIOUS INSTALL and a
# function this file defines below the line that calls it does not exist yet -
# this is a script, not a module, and its definitions happen in order.

function Get-QuotedValue {
    param([string]$Line)
    $open  = $Line.IndexOf("'")
    $close = $Line.LastIndexOf("'")
    if ($open -lt 0 -or $close -le $open) { return '' }
    return $Line.Substring($open + 1, $close - $open - 1)
}

function Get-FirstLine {
    param([string]$Path)
    $text = ''
    try { $text = [string](Get-Content -LiteralPath $Path -Raw -ErrorAction Stop) } catch { return '' }
    foreach ($line in ($text -split '\r?\n')) {
        if ($line.Trim()) { return $line.Trim() }
    }
    return ''
}

# THE SIDECAR LOG IS THE LAST WITNESS A DATA ROOT KEEPS. slog's text format
# writes every attribute as key=value, and the sidecar's logger carries
# peer=<identity> on EVERY line (go/internal/sidecar/sidecar.go, Logger.With);
# its "sidecar: listening" line carries relay=<wss url> beside it. No line ever
# carries a secret - the credential is reported as "configured" and nothing else.
#
# The read is bounded on purpose. A log generation can reach 100 MiB
# (go/internal/logging), so this reads the head and the tail of at most a few
# files: the head holds the startup banner of whatever ran first, the tail holds
# whatever ran last, and a folder that has held two worlds shows both.
$SidecarLogEndLines  = 500
$SidecarLogFileLimit = 12
$script:sidecarLogCandidates = @()
# Whether any line carried a peer= at all, even one this installer would not use.
# It answers a different question from the candidate list: not "which world is
# this" but "did a sidecar ever run here".
$script:sidecarLogSawPeer = $false

function Get-LogAttribute {
    param([string]$Line, [string]$Name)
    $match = [regex]::Match($Line, '(?:^|\s)' + [regex]::Escape($Name) + '=(?:"([^"]*)"|([^\s]+))')
    if (-not $match.Success) { return '' }
    if ($match.Groups[1].Success) { return $match.Groups[1].Value }
    return $match.Groups[2].Value
}

function Find-SidecarLogIdentities {
    param([string]$Root)
    $logsDir = Join-Path $Root 'logs'
    if (-not (Test-Path -LiteralPath $logsDir -PathType Container)) { return @() }
    $found = New-Object System.Collections.ArrayList
    $byPeer = @{}
    $files = @(Get-ChildItem -LiteralPath $logsDir -Filter 'sidecar.log*' -File -ErrorAction SilentlyContinue |
               Sort-Object -Property Name | Select-Object -First $SidecarLogFileLimit)
    foreach ($file in $files) {
        $lines = @()
        try {
            $lines += @(Get-Content -LiteralPath $file.FullName -TotalCount $SidecarLogEndLines -ErrorAction Stop)
            $lines += @(Get-Content -LiteralPath $file.FullName -Tail $SidecarLogEndLines -ErrorAction Stop)
        } catch {
            continue
        }
        foreach ($line in $lines) {
            if (-not $line) { continue }
            $peer = Get-LogAttribute $line 'peer'
            if (-not $peer) { continue }
            $script:sidecarLogSawPeer = $true
            if ($peer.Length -gt 256 -or $peer -notmatch '^[\x21-\x7e]+$') { continue }
            $relay = Get-LogAttribute $line 'relay'
            if ($relay -notmatch '^wss://[^\s]+$') { $relay = '' }
            if ($byPeer.ContainsKey($peer)) {
                if ($relay -and -not $byPeer[$peer].relayUrl) { $byPeer[$peer].relayUrl = $relay }
                continue
            }
            $message = Get-LogAttribute $line 'msg'
            $where = $file.FullName
            if ($message) { $where = "$($file.FullName) (`"$message`")" }
            $entry = [pscustomobject]@{ peerId = $peer; relayUrl = $relay; source = $where }
            $byPeer[$peer] = $entry
            [void]$found.Add($entry)
        }
    }
    return @($found)
}

function New-WorldIdentity {
    param([string]$PeerId, [string]$RelayUrl, [string]$Source, [bool]$Proven)
    return [pscustomobject]@{ peerId = $PeerId; relayUrl = $RelayUrl; source = $Source; proven = $Proven }
}

function Find-WorldIdentity {
    param([string]$Root, [string[]]$ProgramRoots, [string]$Secret)

    $recordFile = Join-Path $Root $RecordName
    $record = Read-JsonFile $recordFile
    $recordRoot = Get-JsonField $record 'dataRoot'
    if ((Get-JsonField $record 'peerId') -and
        ((-not $recordRoot) -or (Test-SamePath $recordRoot $Root))) {
        return (New-WorldIdentity (Get-JsonField $record 'peerId') (Get-JsonField $record 'relayUrl') $recordFile $true)
    }

    if ($Secret) {
        $pendingFile = Join-Path $Root 'enrollment-pending.json'
        $pending = Read-JsonFile $pendingFile
        if ((Get-JsonField $pending 'format') -eq 'bibites-multiverse/enrollment-pending/1' -and
            (Get-JsonField $pending 'secret') -ceq $Secret) {
            $pendingId = [guid]::Empty
            if ([guid]::TryParse((Get-JsonField $pending 'installId'), [ref]$pendingId)) {
                return (New-WorldIdentity ('public-' + $pendingId.ToString('N')) '' $pendingFile $true)
            }
        }
    }

    foreach ($programRoot in $ProgramRoots) {
        if (-not $programRoot) { continue }
        $profilesDir = Join-Path $programRoot $ProfilesDirName
        if (Test-Path -LiteralPath $profilesDir -PathType Container) {
            $profileFiles = @(Get-ChildItem -LiteralPath $profilesDir -Filter '*.json' -File -ErrorAction SilentlyContinue |
                              Sort-Object -Property Name)
            foreach ($profileFile in $profileFiles) {
                $profileData = Read-JsonFile $profileFile.FullName
                if ((Test-SamePath (Get-JsonField $profileData 'dataRoot') $Root) -and
                    (Get-JsonField $profileData 'peerId')) {
                    return (New-WorldIdentity (Get-JsonField $profileData 'peerId') `
                                              (Get-JsonField $profileData 'relayUrl') $profileFile.FullName $false)
                }
            }
        }
        $startScript = Join-Path $programRoot $StartName
        if (Test-Path -LiteralPath $startScript -PathType Leaf) {
            $scriptRoot  = ''
            $scriptPeer  = ''
            $scriptRelay = ''
            foreach ($line in (Get-Content -LiteralPath $startScript -ErrorAction SilentlyContinue)) {
                $text = $line.Trim()
                if     ($text.StartsWith('$DataRoot')) { $scriptRoot  = Get-QuotedValue $text }
                elseif ($text.StartsWith('$PeerId'))   { $scriptPeer  = Get-QuotedValue $text }
                elseif ($text.StartsWith('$RelayUrl')) { $scriptRelay = Get-QuotedValue $text }
            }
            if ((Test-SamePath $scriptRoot $Root) -and $scriptPeer) {
                return (New-WorldIdentity $scriptPeer $scriptRelay $startScript $false)
            }
        }
    }

    # The identity beside the journal, and the map it dials. Both are written by
    # this installer and kept by an uninstall that keeps the world's data, so a
    # private map survives a removal exactly as the public one does.
    $peerIdFile = Join-Path (Join-Path $Root 'data') 'peer-id'
    $storedPeerId = Get-FirstLine $peerIdFile
    if ($storedPeerId) {
        $storedRelay = Get-FirstLine (Join-Path (Join-Path $Root 'data') 'relay-url')
        return (New-WorldIdentity $storedPeerId $storedRelay $peerIdFile $false)
    }

    # Last: what this world said about itself while it was running. ONE identity
    # in the logs is an answer; two is a folder that has held two worlds, and
    # only the person who ran them can say which one it is now - so the list is
    # kept for the refusal to print rather than guessed between.
    $script:sidecarLogCandidates = @(Find-SidecarLogIdentities $Root)
    if ($script:sidecarLogCandidates.Count -eq 1) {
        $logged = $script:sidecarLogCandidates[0]
        return (New-WorldIdentity $logged.peerId $logged.relayUrl $logged.source $false)
    }

    return $null
}

# The pending record beside a completed identity has to be that same identity's.
# Two identities in one data root is a state only a person can settle, and
# nothing is changed on the way to saying so.
function Assert-PendingAgrees {
    param([string]$PendingPath, [string]$PeerId, [string]$Secret)
    if (-not $PendingPath) { return }
    if (-not (Test-Path -LiteralPath $PendingPath -PathType Leaf)) { return }
    $pending = Read-JsonFile $PendingPath
    $pendingPeerId = ''
    $pendingInstallId = [guid]::Empty
    if ([guid]::TryParse((Get-JsonField $pending 'installId'), [ref]$pendingInstallId)) {
        $pendingPeerId = 'public-' + $pendingInstallId.ToString('N')
    }
    if ((Get-JsonField $pending 'format') -ne 'bibites-multiverse/enrollment-pending/1' -or
        $pendingPeerId -cne $PeerId -or
        (Get-JsonField $pending 'secret') -cne $Secret) {
        Stop-Setup ("The world in $DataRoot is $PeerId, and $PendingPath is a half-finished " +
                    "enrollment for a different one. Neither file was changed. Delete the pending " +
                    "record only if you are sure the world you want is $PeerId; ask the operator " +
                    "otherwise.") 'INS-ENROLL'
    }
}

# What is in peer-secret.txt is decided from the WHOLE file, not from its first
# line: a credential that starts with a blank line is still a credential, and
# treating it as absent would delete it and spend a second identity.
$existingSecret = ''
$existingSecretHeld = Test-Path -LiteralPath $credentialPath -PathType Leaf
if ($existingSecretHeld) {
    $credentialText = ''
    try {
        $credentialText = [string](Get-Content -LiteralPath $credentialPath -Raw -ErrorAction Stop)
    } catch {
        Stop-Setup ("$credentialPath holds this data root's world credential, and this account cannot read " +
                    "it: $($_.Exception.Message) Run setup from the account that installed this world, or " +
                    "move that file aside to start a new one - the old world keeps its place on the map " +
                    "until you ask the operator to release it (docs/participant/leave.md).") 'INS-ENROLL'
    }
    if (-not $credentialText.Trim()) {
        # An EMPTY file holds no world and nothing that can be lost. It is the one
        # state that is treated as absent, and it is written like a new one.
        $existingSecretHeld = $false
    } else {
        foreach ($line in ($credentialText -split '\r?\n')) {
            if ($line.Trim()) { $existingSecret = $line.Trim(); break }
        }
        if ($existingSecret.Length -lt 32 -or $existingSecret.Length -gt 256 -or
            $existingSecret -notmatch '^[\x21-\x7e]+$') {
            Stop-Setup ("$credentialPath has something in it that is not a map credential: a secret " +
                        "is 32 to 256 printable characters with no spaces, and this is " +
                        "$($existingSecret.Length). Nothing was changed, because a file this " +
                        "installer cannot read is not a file it may overwrite. If a world's secret " +
                        "belongs there, put it back whole. If it is something else, rename it to " +
                        "peer-secret.txt.old and run setup again.") 'INS-ENROLL'
        }
    }
}

# Bounded and named, in this order. The last two are here because an advanced
# ZIP is usually unpacked beside the data it writes, and the kit that made this
# world may be long gone from -InstallRoot: a profile or a start script only
# counts when it names THIS data root, so a wider net would add risk without
# adding an answer.
$programRoots = @()
if ($InstallRoot) { $programRoots += $InstallRoot }
$programRoots += $Here
if ($env:LOCALAPPDATA) { $programRoots += (Join-Path $env:LOCALAPPDATA 'Programs\Bibites Multiverse') }
$programRoots += $DataRoot
$dataRootParent = Split-Path -Parent $DataRoot
if ($dataRootParent) { $programRoots += $dataRootParent }
$existingIdentity = Find-WorldIdentity $DataRoot $programRoots $existingSecret
$existingPeerId = ''
$existingSource = ''
$existingProven = $false
if ($existingIdentity) {
    $existingPeerId = [string]$existingIdentity.peerId
    $existingSource = [string]$existingIdentity.source
    $existingProven = [bool]$existingIdentity.proven
}

# DID A SIDECAR EVER RUN IN THIS DATA ROOT? Its first start, in this order
# (go/internal/sidecar/sidecar.go, New): creates <data root>\data; mints
# data\contract-a.token, mode 0600, BEFORE the listener binds; opens
# data\journal\journal.log, which journal.Open creates even when there is
# nothing to replay; opens data\genomes; then writes data\peer-id and
# data\listen.addr, and data\slot and data\position once the map grants a
# place. THIS INSTALLER CREATES <data root>\data EMPTY and writes nothing into
# it until the credential is settled, so ANY entry inside it is a sidecar's, and
# a peer= in a sidecar log says the same thing from the other side.
#
# <data root>\logs is deliberately not part of this: the graphical setup writes
# its own install-<utc>.log there, and that is not a world having run.
function Test-WorldEverRan {
    param([string]$Root)
    if ($script:sidecarLogSawPeer) { return $true }
    $dataPath = Join-Path $Root 'data'
    if (Test-Path -LiteralPath $dataPath -PathType Container) {
        $entries = @(Get-ChildItem -LiteralPath $dataPath -Force -ErrorAction SilentlyContinue)
        if ($entries.Count -gt 0) { return $true }
    }
    return $false
}

# THE ORPHAN OF AN INTERRUPTED INSTALL. A secret with no name anywhere is
# normally a refusal, because the world it belongs to is somewhere and only a
# person can say where. But a secret in a data root NO SIDECAR EVER RAN IN
# belongs to no world at all: the earlier install got as far as the credential
# and stopped, so nothing ever claimed a slot, nothing was ever addressed to it,
# and there is nothing on the map to strand. That is not a choice to put to
# somebody - it is a decision this installer can make. It still never deletes:
# the secret is renamed aside, protected, and named on screen.
if ($existingSecretHeld -and (-not $existingIdentity) -and (-not (Test-WorldEverRan $DataRoot))) {
    $orphanPath = "$credentialPath." + (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '.orphan'
    Move-Item -LiteralPath $credentialPath -Destination $orphanPath -Force
    [void](Protect-UserFile $orphanPath 'the orphaned map credential')
    Write-Host ""
    Say "$credentialPath was an orphan: an earlier install got that secret and stopped before"
    Say "this world ever ran, so no sidecar has ever written in $DataRoot, that identity never"
    Say "claimed a place on the map, and nothing on this machine can name it. It is KEPT, as"
    Say "$orphanPath, and this install enrolls a new identity below."
    Write-Host ""
    $existingSecretHeld = $false
    $existingSecret = ''
}

# The relay this package ships, when it ships one. It is read even on a private
# install, because what the certificate step says has to follow the map this
# world actually dials rather than the branch that chose it.
$packagedRelayUrl = ''
$publicMapPath = Join-Path $Here $PublicMapName
if (Test-Path -LiteralPath $publicMapPath -PathType Leaf) {
    $packagedMap = Read-JsonFile $publicMapPath
    if ((Get-JsonField $packagedMap 'format') -eq 'bibites-multiverse/public-map/1') {
        $packagedRelayUrl = Get-JsonField $packagedMap 'relayUrl'
    }
}
$usePublicMap = (-not $JoinStringFile) -and (-not $RelayUrl) -and
                (Test-Path -LiteralPath $publicMapPath -PathType Leaf)
if ($usePublicMap) {
    $pendingEnrollmentPath = Join-Path $DataRoot 'enrollment-pending.json'
    try {
        $publicMap = Get-Content -LiteralPath $publicMapPath -Raw | ConvertFrom-Json
    } catch {
        Stop-Setup "$PublicMapName is not valid JSON." 'INS-ENROLL'
    }
    foreach ($required in @('format', 'enrollmentUrl', 'relayUrl')) {
        if ($publicMap.PSObject.Properties.Match($required).Count -eq 0) {
            Stop-Setup "$PublicMapName is missing its '$required' field." 'INS-ENROLL'
        }
    }
    if ($publicMap.format -ne 'bibites-multiverse/public-map/1') {
        Stop-Setup "$PublicMapName has an unsupported format." 'INS-ENROLL'
    }
    $enrollmentUrl = [string]$publicMap.enrollmentUrl
    $publicRelayUrl = [string]$publicMap.relayUrl
    if ($enrollmentUrl -notmatch '^https://[^\s]+$') {
        Stop-Setup "$PublicMapName does not contain a secure HTTPS enrollment address." 'INS-ENROLL'
    }
    if ($publicRelayUrl -notmatch '^wss://[^\s]+$') {
        Stop-Setup "$PublicMapName does not contain a secure WSS relay address." 'INS-ENROLL'
    }

    # THE WORLD ALREADY IN THIS DATA ROOT KEEPS ITS IDENTITY. Enrollment is for a
    # data root with no world in it. Everything else - an upgrade, a repair, a
    # changed runtime, an install over an earlier one - adopts what is here, and
    # neither asks the map for a second identity nor rewrites the secret file.
    if ($existingSecretHeld) {
        if (-not $existingIdentity) {
            Write-Host ""
            Say "$credentialPath holds a world's secret, and nothing in $DataRoot says which world"
            Say "it belongs to. Nothing was changed. Two ways on, and only you can choose:"
            Say ""
            if ($sidecarLogCandidates.Count -gt 1) {
                Say "  1. NAME THE WORLD. THIS INSTALLER READ THE SIDECAR LOG in $logDir and it"
                Say "     names more than one world, so it will not choose between them:"
                foreach ($candidate in $sidecarLogCandidates) {
                    Say "       $($candidate.peerId)"
                    Say "         from $($candidate.source)"
                }
                Say "     Put the ONE this folder is now into $DataRoot\data\peer-id, on one line,"
                Say "     and run setup again. The newest of them is usually the last world this"
                Say "     folder ran; its journal is the one in $dataDir."
            } else {
                Say "  1. NAME THE WORLD, and this install keeps it. THIS INSTALLER ALREADY LOOKED"
                Say "     in the places that hold it: this data root, the folder this kit was"
                Say "     unpacked into, %LOCALAPPDATA%\Programs\Bibites Multiverse, and the"
                Say "     sidecar log in $logDir. None of them names a world. It is also in an"
                Say ('     older kit''s profiles\default.json and in the $PeerId line of its ' + $StartName + ',')
                Say "     wherever that kit was unpacked. It looks like"
                Say "     public-0123456789abcdef0123456789abcdef. Put that one line into"
                Say "     $DataRoot\data\peer-id and run setup again."
            }
            Say "  2. START A NEW WORLD. Rename $credentialPath to peer-secret.txt.old, or move"
            Say "     $DataRoot aside, and run setup again. The old world then keeps its place on"
            Say "     the map with nobody in it until you ask the operator to release it."
            Stop-Setup ("The secret in $credentialPath was NOT overwritten: it is the only " +
                        "recoverable half of an identity this map cannot print again. Name the " +
                        "world in $DataRoot\data\peer-id, or move that secret aside for a new " +
                        "world - the two choices above, and docs/participant/leave.md.") 'INS-ENROLL'
        }
        $adoptedPeerId   = $existingPeerId
        $adoptedRelayUrl = [string]$existingIdentity.relayUrl
        if (-not $adoptedRelayUrl) {
            if ($adoptedPeerId -cmatch '^public-[0-9a-f]{32}$') {
                $adoptedRelayUrl = $publicRelayUrl
            } else {
                Stop-Setup ("$DataRoot holds the world $adoptedPeerId, and nothing here says which " +
                            "map it is on: that identity is not one this package's public map " +
                            "issues, and $DataRoot\data\relay-url is not there either. Nothing was " +
                            "changed. Put that map's wss:// address into $DataRoot\data\relay-url " +
                            "on one line and run setup again, or run setup from a console with " +
                            "-JoinStringFile <file> holding the one line your operator sent - or " +
                            "move $DataRoot aside to start a new world on the public map.") 'INS-ENROLL'
            }
        }
        Assert-PendingAgrees $pendingEnrollmentPath $adoptedPeerId $existingSecret
        $RelayUrl   = $adoptedRelayUrl
        $credential = $adoptedPeerId + '.' + $existingSecret
        $adoptedIdentity = $true
        Say "reusing the map identity already in $DataRoot (peer $adoptedPeerId)"
        Say "read from $($existingSource): no identity was created and no slot was spent"
        if ($adoptedRelayUrl -cne $publicRelayUrl) {
            Say "that world is on $adoptedRelayUrl, which is not the map this package ships. It"
            Say "stays where it is; -JoinStringFile is what moves a data root to another map."
        }
    } elseif ($existingIdentity -and (-not $ReplaceWorldIdentity)) {
        # The name is here and the secret is not. NOTHING RECOVERS A SECRET, so the
        # only way on is a second place on the map - and that is a decision, not a
        # step. It is refused here rather than taken quietly, because on the
        # graphical path there is no other moment to refuse it in.
        Write-Host ""
        Say "$DataRoot is the world $existingPeerId - $existingSource says so - and its secret,"
        Say "$credentialPath, is gone. Nothing recovers it: the map keeps a verifier and cannot"
        Say "print a secret again. Write $existingPeerId down. Then, two ways on:"
        Say ""
        Say "  1. GET THAT WORLD'S PLACE BACK, with a SLOT HANDOVER. It keeps the slot, the"
        Say "     position and everything addressed to it, and gives you a NEW identity with a"
        Say "     freshly minted credential. Its operator sends you one join string; run setup"
        Say "     from a console with -JoinStringFile <file> -ReplaceWorldIdentity."
        Say "  2. START A NEW WORLD HERE, and leave $existingPeerId dark on the map until its"
        Say "     operator releases it. Run setup from a console with -ReplaceWorldIdentity, or"
        Say "     rename $DataRoot\data\peer-id to peer-id.old first; either way the old name is"
        Say "     kept in $DataRoot\data\peer-id.previous for you."
        Stop-Setup ("A new identity over this world is either its slot handed back to you by a " +
                    "slot handover, or a SECOND place on the map that strands $existingPeerId, " +
                    "and nothing on this " +
                    "machine can tell which. -ReplaceWorldIdentity says take it - the two choices " +
                    "above, and docs/participant/leave.md.") 'INS-ENROLL'
    } elseif (($sidecarLogCandidates.Count -gt 1) -and (-not $ReplaceWorldIdentity)) {
        # No secret, no single name - but this folder's own logs say it has run
        # more than one world. Enrolling here would be a third identity over
        # somebody's journal, so it is the same decision as the one above.
        Write-Host ""
        Say "$credentialPath is gone, so no world here can be kept. The sidecar log in $logDir"
        Say "names more than one world that has run in $DataRoot"
        foreach ($candidate in $sidecarLogCandidates) {
            Say "       $($candidate.peerId)"
            Say "         from $($candidate.source)"
        }
        Say "Write those names down: their slots stay reserved until their operator releases"
        Say "them. Then run setup from a console with -ReplaceWorldIdentity to take a new"
        Say "identity here, or with -JoinStringFile <file> -ReplaceWorldIdentity to apply a"
        Say "slot handover for one of them."
        Stop-Setup ("A new identity here is a new place on the map over the journal of a world " +
                    "this folder has already run, and its secret is gone, so nothing here can " +
                    "keep it. -ReplaceWorldIdentity says take it anyway - " +
                    "docs/participant/leave.md.") 'INS-ENROLL'
    } elseif ($existingIdentity -or ($sidecarLogCandidates.Count -gt 1)) {
        # -ReplaceWorldIdentity: the participant has said, in as many words, that
        # this folder takes a different identity. Say what it costs anyway, and
        # keep the old name - it is what an operator needs to release the slot.
        Write-Host ""
        if ($existingPeerId) {
            Write-Host ("  -ReplaceWorldIdentity: the world that used $DataRoot was $existingPeerId, " +
                        "and its secret is gone.") -ForegroundColor Yellow
        } else {
            Write-Host "  -ReplaceWorldIdentity: this folder's own logs name worlds that have run here," -ForegroundColor Yellow
            Write-Host "  and the secret of every one of them is gone:" -ForegroundColor Yellow
            foreach ($candidate in $sidecarLogCandidates) {
                Write-Host "    $($candidate.peerId)" -ForegroundColor Yellow
            }
        }
        Write-Host "  This install therefore takes a NEW identity, which is a second place on the map." -ForegroundColor Yellow
        Write-Host "  The old one keeps its slot, with nobody in it, until you ask the operator to" -ForegroundColor Yellow
        Write-Host "  release it or to hand it to this installation - docs/participant/leave.md." -ForegroundColor Yellow
        if ($existingPeerId) {
            Write-Host ("  Its name is kept in $DataRoot\data\peer-id.previous, and it is what the " +
                        "operator needs.") -ForegroundColor Yellow
        }
        Write-Host ""
        # A leftover pending record here is a new identity already half-taken, and
        # retrying it is what spends nothing extra. It cannot be the world named on
        # disk - that world has no secret - so it is announced, not refused.
        $leftoverPending = Read-JsonFile $pendingEnrollmentPath
        if ((Get-JsonField $leftoverPending 'format') -eq 'bibites-multiverse/enrollment-pending/1') {
            Say "an enrollment already begun in this folder is finished rather than started again"
        }
    }

    if (-not $adoptedIdentity) {
        $installId = ''
        $enrollmentSecret = ''
        if (Test-Path -LiteralPath $pendingEnrollmentPath -PathType Leaf) {
            try {
                $pending = Get-Content -LiteralPath $pendingEnrollmentPath -Raw | ConvertFrom-Json
            } catch {
                Stop-Setup ("$pendingEnrollmentPath is not valid JSON. Remove it only if you want " +
                            "the installer to create a different map identity.") 'INS-ENROLL'
            }
            if ($pending.PSObject.Properties.Match('format').Count -eq 0 -or
                $pending.PSObject.Properties.Match('installId').Count -eq 0 -or
                $pending.PSObject.Properties.Match('secret').Count -eq 0 -or
                $pending.format -ne 'bibites-multiverse/enrollment-pending/1') {
                Stop-Setup ("$pendingEnrollmentPath is not an enrollment record this installer " +
                            "can use. Remove it only if you want a different map identity.") 'INS-ENROLL'
            }
            $installId = [string]$pending.installId
            $enrollmentSecret = [string]$pending.secret
            Say 'retrying the pending public-map enrollment'
        } else {
            $installId = [guid]::NewGuid().ToString('D')
            $secretBytes = New-Object byte[] 32
            $random = [System.Security.Cryptography.RandomNumberGenerator]::Create()
            try { $random.GetBytes($secretBytes) } finally { $random.Dispose() }
            $enrollmentSecret = -join @($secretBytes | ForEach-Object { $_.ToString('x2') })
            [ordered]@{
                format    = 'bibites-multiverse/enrollment-pending/1'
                installId = $installId
                secret    = $enrollmentSecret
            } | ConvertTo-Json | Set-Content -LiteralPath $pendingEnrollmentPath -Encoding ASCII
        }

        $parsedInstallId = [guid]::Empty
        if (-not [guid]::TryParse($installId, [ref]$parsedInstallId) -or
            $enrollmentSecret -notmatch '^[0-9a-f]{64}$') {
            Stop-Setup ("$pendingEnrollmentPath contains an invalid map identity. Remove it only " +
                        "if you want the installer to create a different identity.") 'INS-ENROLL'
        }
        [void](Protect-UserFile $pendingEnrollmentPath 'the pending map credential' -Required)
        $expectedPeerId = 'public-' + $parsedInstallId.ToString('N')
        $request = [ordered]@{
            format    = 'bibites-multiverse/enrollment-request/1'
            installId = $parsedInstallId.ToString('D')
            secret    = $enrollmentSecret
            release   = $Release
        } | ConvertTo-Json -Compress
        Say "requesting a unique identity from $enrollmentUrl"
        try {
            $response = Invoke-RestMethod -Method Post -Uri $enrollmentUrl -ContentType 'application/json' `
                                          -Body $request -TimeoutSec 30
        } catch {
            Stop-Setup ("The public map did not create this installation's identity. The pending " +
                        "identity was kept, so running the installer again is a safe retry.") 'INS-ENROLL'
        }
        foreach ($required in @('format', 'relayUrl', 'peerId')) {
            if ($response.PSObject.Properties.Match($required).Count -eq 0) {
                Stop-Setup "The public map returned an incomplete enrollment response." 'INS-ENROLL'
            }
        }
        if ($response.format -ne 'bibites-multiverse/enrollment-response/1' -or
            [string]$response.relayUrl -ne $publicRelayUrl -or
            [string]$response.peerId -ne $expectedPeerId) {
            Stop-Setup "The public map returned an enrollment response for a different identity or relay." 'INS-ENROLL'
        }
        $RelayUrl = $publicRelayUrl
        $credential = $expectedPeerId + '.' + $enrollmentSecret
        $usedAutomaticEnrollment = $true
    }
} elseif ((-not $JoinStringFile) -and $RelayUrl -and $existingSecretHeld -and $existingIdentity) {
    # -RelayUrl alone, over a data root that already holds a world: the join
    # string is not needed, because the secret half is already here. This is the
    # console remedy for a private-map world whose install record was lost - and
    # it NAMES the map rather than moving the world to one. The same identity and
    # secret presented to a different relay is a different world or an authentication
    # failure, and the journal in this folder belongs to neither.
    $knownRelayUrl = [string]$existingIdentity.relayUrl
    if ($knownRelayUrl -and ($knownRelayUrl -cne $RelayUrl)) {
        Stop-Setup ("$DataRoot holds the world $existingPeerId on $knownRelayUrl - $existingSource " +
                    "says so - and -RelayUrl names $RelayUrl instead. Nothing was changed. A world " +
                    "does not move between maps: its slot, its position and its journal are on the " +
                    "one it joined. Run this again without -RelayUrl to keep it, or use -DataRoot " +
                    "for a new world on the other map.") 'INS-ENROLL'
    }
    Assert-PendingAgrees (Join-Path $DataRoot 'enrollment-pending.json') $existingPeerId $existingSecret
    $credential = $existingPeerId + '.' + $existingSecret
    $adoptedIdentity = $true
    Say "reusing the map identity already in $DataRoot (peer $existingPeerId)"
    Say "read from $($existingSource): no identity was created and no slot was spent"
} else {
    $joinString = Read-JoinString $JoinStringFile
    if (-not $joinString) { Stop-Setup "No join string was given, so this world has no identity on any map." 'INS-JOINSTRING' }

    $fields = @($joinString -split '\s+' | Where-Object { $_ })
    if ($fields.Count -eq 3 -and $fields[0] -eq 'multiverse-join/1') {
        $RelayUrl   = $fields[1]
        $credential = $fields[2]
    } elseif ($fields.Count -eq 1) {
        $credential = $fields[0]
        if (-not $RelayUrl) {
            Stop-Setup ("That is the identity half on its own, with no relay address. Either paste the " +
                        "whole one-line join string - it starts with 'multiverse-join/1' - or run this " +
                        "script again with -RelayUrl wss://<relay-host>/contract-b/v4.") 'INS-JOINSTRING'
        }
    } else {
        Stop-Setup ("That is not a join string this installer can read. The one-line form is three " +
                    "parts: 'multiverse-join/1', the wss:// relay address, and your identity and " +
                    "secret joined by a dot. Ask your operator to send the 'one line' from the block " +
                    "their relay printed.") 'INS-JOINSTRING'
    }
}

if ($RelayUrl -notmatch '^wss://[^\s]+$') {
    if ($RelayUrl -match '^ws://') {
        Stop-Setup ("The relay address is ws://, which is not encrypted. This wire is always " +
                    "wss:// and there is no fallback anywhere in this software: a relay that " +
                    "answers ws:// off loopback refuses the connection rather than serving it. " +
                    "Ask your operator for the wss:// address.") 'INS-JOINSTRING'
    }
    Stop-Setup "The relay address '$RelayUrl' is not a wss:// URL." 'INS-JOINSTRING'
}

# The two halves split on the LAST dot: an identity may legally contain one and a
# secret may not. This is exactly the wire's own rule (contract-b-m4.md §3.1).
$dot = $credential.LastIndexOf('.')
if ($dot -le 0 -or $dot -eq $credential.Length - 1) {
    Stop-Setup ("The identity half and the secret half are joined by a dot, and this value has no " +
                "usable one. Paste the whole 'one line' your operator sent.") 'INS-JOINSTRING'
}
$peerId = $credential.Substring(0, $dot)
$secret = $credential.Substring($dot + 1)

if ($secret.Length -lt 32 -or $secret.Length -gt 256) {
    Stop-Setup ("The secret half is $($secret.Length) characters. It must be 32 to 256. Nothing was " +
                "written; ask your operator to re-send the join string exactly as their relay printed it.") 'INS-JOINSTRING'
}
if ($secret -notmatch '^[\x21-\x7e]+$') {
    Stop-Setup "The secret half must be printable ASCII with no spaces. Nothing was written." 'INS-JOINSTRING'
}
if ($peerId -notmatch '^[\x21-\x7e]+$') {
    Stop-Setup "The identity half must be printable ASCII with no spaces. Nothing was written." 'INS-JOINSTRING'
}

if ($adoptedIdentity) {
    # The file on disk IS the credential this install adopted. Its CONTENTS are
    # never rewritten - no delete, no write, no truncate - so a failure on this
    # path can never cost somebody the only copy of their secret. The permission
    # is re-applied in place, because a repair that leaves a loose credential
    # loose is not a repair.
    Say "$credentialPath was left exactly as it is"
    [void](Protect-UserFile $credentialPath 'the map credential')
} else {
    # A HANDOVER MINTS A NEW IDENTITY (contract-b-m4.md §7.5: --handover-slot
    # names a new peerId and a freshly minted credential), so a join string that
    # does not match the world already here is either that recovery or a mistake,
    # and nothing on this machine can tell them apart. Both are gated, and the one
    # destructive act in this script - writing over a secret - never happens on
    # the word of a file anybody can edit.
    $identityChanges = ($existingPeerId -and ($peerId -cne $existingPeerId))
    if ($existingSecretHeld -and (-not $existingPeerId)) {
        Stop-Setup ("$credentialPath already holds a secret, and nothing in $DataRoot says which " +
                    "world it belongs to. Nothing was changed, because that secret is the only " +
                    "recoverable half of an identity no map can print again. If this join string " +
                    "is that world's slot handed back to you, the file is already spent: rename it " +
                    "to peer-secret.txt.old and run this again. Otherwise use -DataRoot for a new " +
                    "world.") 'INS-ENROLL'
    }
    if ($identityChanges -and (-not $ReplaceWorldIdentity)) {
        Stop-Setup ("$DataRoot is the world $existingPeerId (named in $existingSource), and that " +
                    "join string is a different identity, $peerId. Nothing was changed. If your " +
                    "operator handed this world's slot over, that IS a new identity and " +
                    "-ReplaceWorldIdentity applies it here, keeping the journal, the slot and the " +
                    "position. If they did not, this would leave $existingPeerId dark on the map " +
                    "with a second world over its journal: use -DataRoot for the new world " +
                    "instead. See docs/participant/leave.md.") 'INS-ENROLL'
    }
    if ($existingSecretHeld -and (-not $identityChanges) -and (-not $existingProven) -and
        ($secret -cne $existingSecret)) {
        Stop-Setup ("$existingSource is the only thing that says $DataRoot is the world $peerId, " +
                    "and it is an ordinary text file that nothing binds to the secret in " +
                    "$credentialPath. That is enough to keep a world and not enough to overwrite " +
                    "one, so nothing was changed. If that secret is spent, rename $credentialPath " +
                    "to peer-secret.txt.old and run this again with the same join string. If it is " +
                    "the live one, run setup with no join string at all and it is kept as it is.") 'INS-ENROLL'
    }
    if ($existingSecretHeld -and ($secret -cne $existingSecret)) {
        # The replaced secret is kept beside the new one rather than deleted: a
        # join string that turns out to be the wrong one would otherwise be
        # unrecoverable in exactly the way this whole step exists to prevent.
        $backupPath = "$credentialPath." + (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '.old'
        Copy-Item -LiteralPath $credentialPath -Destination $backupPath -Force
        [void](Protect-UserFile $backupPath 'the replaced map credential')
        Say "the secret it replaces is kept in $backupPath - delete it once this world connects"
    }
    if ($identityChanges) {
        Say "-ReplaceWorldIdentity: $DataRoot was the world $existingPeerId and is now $peerId."
        Say "If that came from a slot handover, its slot and position came with it and nothing is"
        Say "stranded. If it did not, $existingPeerId is dark on the map until its operator"
        Say "releases it - docs/participant/leave.md."
    } elseif ($existingSecretHeld -and $existingPeerId) {
        Say "the same world, $peerId (named in $existingSource)"
    }
    # Written fresh every time: re-writing over an already protected file makes the
    # permission change below need a privilege an ordinary account does not have.
    Remove-Item -LiteralPath $credentialPath -Force -ErrorAction SilentlyContinue
    Set-Content -LiteralPath $credentialPath -Value $secret -Encoding ASCII
    [void](Protect-UserFile $credentialPath 'the map credential')
}
$secret = ''
$existingSecret = ''

# The identity and the map beside the journal they belong to. The sidecar keeps
# peer-id itself (contract-b-m4.md §7.4) and reads it when nothing passes
# --peer-id; both are written here because an uninstall that keeps this world's
# data keeps them too, so installing again over it finds the world whole - on a
# private map exactly as on the public one.
$peerIdFile   = Join-Path $dataDir 'peer-id'
$relayUrlFile = Join-Path $dataDir 'relay-url'
$storedPeerId = Get-FirstLine $peerIdFile
if ($storedPeerId -cne $peerId) {
    if ($storedPeerId) {
        # The name this folder used to answer to. An operator needs that string to
        # release or hand over the slot it stranded, and nothing else keeps it.
        Set-Content -LiteralPath (Join-Path $dataDir 'peer-id.previous') -Value $storedPeerId -Encoding ASCII
        Say "the world this folder used to be, $storedPeerId, is kept in $dataDir\peer-id.previous"
        Say "It is dark on the map until its operator releases it - docs/participant/leave.md."
    }
    Set-Content -LiteralPath $peerIdFile -Value $peerId -Encoding ASCII
}
if ((Get-FirstLine $relayUrlFile) -cne $RelayUrl) {
    Set-Content -LiteralPath $relayUrlFile -Value $RelayUrl -Encoding ASCII
}

Say "your world's identity on this map: $peerId"
Say "the relay it dials: $RelayUrl"
$onPackagedMap = ($packagedRelayUrl -and ($RelayUrl -ceq $packagedRelayUrl))
if (-not $onPackagedMap) {
    Write-Host ""
    Say "IF YOU LOSE THAT SECRET there is no software recovery: the relay keeps a verifier"
    Say "and cannot print it again. The only way back is to ask the operator for a slot"
    Say "handover, which mints a fresh one. Your slot, your position and everything"
    Say "addressed to you survive that - see docs/participant/leave.md."
}
if ($JoinStringFile) {
    Write-Host ""
    Say "Delete $JoinStringFile now. It still holds the secret in clear text, and this"
    Say "installer will not delete a file you gave it."
}

# ---------------------------------------------------------------- 7. the certificate

Step "7 of 9 - the relay's certificate"

$caImported   = $false
$caThumbprint = ''
$caStored     = ''

# AN UPGRADE MUST NOT FORGET A CERTIFICATE IT IMPORTED. The record is rewritten
# whole below, and a private-map participant who upgrades without repeating
# -CaFile would have written `imported: false` over the one statement that lets
# the uninstall take that authority back out of their own user store - leaving it
# there for ever, with nothing left on the machine that knows it is there. The
# carry-forward is CHECKED against the store rather than believed: an authority
# somebody has already removed by hand is not claimed again.
if (-not $CaFile -and $previousInstall.Record -and
    $previousInstall.Record.PSObject.Properties.Match('certificate').Count -gt 0 -and
    $previousInstall.Record.certificate) {
    $previousCa = $previousInstall.Record.certificate
    $previousThumb = Get-JsonField $previousCa 'thumbprint'
    if ((Get-JsonField $previousCa 'imported') -eq 'True' -and $previousThumb) {
        $stillThere = @(Get-ChildItem 'Cert:\CurrentUser\Root' -ErrorAction SilentlyContinue |
                        Where-Object { $_.Thumbprint -eq $previousThumb }).Count -gt 0
        if ($stillThere) {
            $caImported   = $true
            $caThumbprint = $previousThumb
            $caStored     = Get-JsonField $previousCa 'storedCopy'
            Say "the certificate this installation imported is still in your own user store,"
            Say "and this record keeps naming it so the uninstall can still take it out."
        }
    }
}

if (-not $CaFile) {
    # What this says follows the map this world DIALS, not the branch that chose
    # it: an adopted private world reaches this step through the public-map path.
    if ($caImported) {
        Say "NOTHING NEW WAS IMPORTED INTO ANY TRUST STORE by this run."
    } else {
        Say "NOTHING WAS IMPORTED INTO ANY TRUST STORE, and nothing needs to be."
    }
    if ($onPackagedMap) {
        Say "A public map's relay presents a certificate signed by an authority Windows"
        Say "already trusts, so your machine checks it the same way it checks a bank's."
        Say "Your sidecar verifies that certificate on every connection, and there is no"
        Say "switch anywhere in this software that skips the check."
        Say "-CaFile exists for a private or LAN map only. If your operator sent you a"
        Say "ca.crt, run this installer again with -CaFile .\ca.crt."
    } else {
        Say "This world dials $RelayUrl, which is not the map this package ships."
        Say "Your sidecar verifies that relay's certificate on every connection, and there"
        Say "is no switch anywhere in this software that skips the check. If that relay"
        Say "presents a certificate an authority Windows already trusts, this is right as"
        Say "it is. If its map's operator sent you a ca.crt, run this installer again with"
        Say "-CaFile .\ca.crt, which imports it into YOUR OWN user store and nowhere else."
    }
} else {
    if (-not (Test-Path $CaFile)) { Stop-Setup "No certificate file at $CaFile." }
    try {
        $ca = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 `
            ((Resolve-Path $CaFile).Path)
    } catch {
        Stop-Setup "$CaFile is not a certificate this machine can read: $($_.Exception.Message)"
    }
    $caThumbprint = $ca.Thumbprint
    Say "subject    : $($ca.Subject)"
    Say "thumbprint : $caThumbprint"
    Say "valid until: $($ca.NotAfter.ToString('u'))"
    if ($ca.NotAfter -lt (Get-Date)) {
        Stop-Setup "That authority expired on $($ca.NotAfter.ToString('u')). Ask your operator for a current one."
    }

    $caStored = Join-Path $DataRoot 'relay-ca.crt'
    Copy-Item -Path $CaFile -Destination $caStored -Force

    Write-Host ""
    Say "WHAT YOU ARE AGREEING TO, because this is the one step that reaches outside"
    Say "the game. This authority goes into YOUR OWN user store - Cert:\CurrentUser\Root"
    Say "- not the machine's. It needs no administrator rights and it affects no other"
    Say "account. While it is there, programs running as you trust anything it signs."
    Say "It was made by one map's operator for one relay. Take it out whenever you like:"
    Say "    .\Uninstall-BibitesMultiverse.ps1"
    Say "or by hand:"
    Say "    Get-ChildItem Cert:\CurrentUser\Root | Where-Object Thumbprint -eq '$caThumbprint' | Remove-Item"
    Write-Host ""

    $already = @(Get-ChildItem 'Cert:\CurrentUser\Root' -ErrorAction SilentlyContinue |
                 Where-Object { $_.Thumbprint -eq $caThumbprint }).Count -gt 0
    if ($already) {
        Say "already trusted here; nothing was imported, and the uninstall will leave it alone."
    } elseif ($SkipCaImport) {
        Say "-SkipCaImport was given, so nothing was imported. Your sidecar will refuse to"
        Say "connect until this authority is trusted. Import it yourself with:"
        Say "    Import-Certificate -FilePath '$caStored' -CertStoreLocation Cert:\CurrentUser\Root"
    } else {
        $importError = ''
        try {
            Import-Certificate -FilePath $caStored -CertStoreLocation 'Cert:\CurrentUser\Root' | Out-Null
        } catch {
            $importError = $_.Exception.Message
        }
        $caImported = @(Get-ChildItem 'Cert:\CurrentUser\Root' -ErrorAction SilentlyContinue |
                        Where-Object { $_.Thumbprint -eq $caThumbprint }).Count -gt 0
        if ($caImported) {
            Say "imported into Cert:\CurrentUser\Root and read back by thumbprint."
        } else {
            Write-Host "The authority was NOT imported." -ForegroundColor Red
            if ($importError) { Say "reason: $importError" }
            Say "Nothing else is wrong: the plugin and the sidecar are installed. Your sidecar"
            Say "verifies the relay's certificate against this machine's trust store and will"
            Say "not connect until this authority is in it. There is no switch that skips the"
            Say "check, on purpose. Import it by hand:"
            Say "    Import-Certificate -FilePath '$caStored' -CertStoreLocation Cert:\CurrentUser\Root"
        }
    }
}

# ---------------------------------------------------------------- 8. the settings

Step "8 of 9 - the settings this install ships with"

# AN UPGRADE KEEPS WHAT THIS INSTALLATION WAS ALREADY SET TO. The graphical setup
# passes no settings at all, so before this every value below was the installer's
# own default - and a participant who had renamed their world, moved its port or
# changed how often it saves got all of it reset by a run whose entire purpose
# was to bring them a newer build.
#
# THE ADOPTION HAPPENS HERE, BEFORE THE VALIDATION AND BEFORE THE NARRATION, so
# an adopted value is checked by the same rules a named one is and the screen
# says what this install actually runs with rather than what its defaults are. A
# value named on this run is never touched: Get-PreviousSetting's first rule.
if ($previousInstall.Present) {
    $World       = [string](Get-PreviousSetting $previousInstall ($PSBoundParameters.ContainsKey('World')) `
                                'world' 'world' $World)
    $SidecarPort = [int](Get-PreviousSetting $previousInstall ($PSBoundParameters.ContainsKey('SidecarPort')) `
                                'sidecarPort' 'sidecarPort' $SidecarPort)
    $ExportEdges = [string](Get-PreviousSetting $previousInstall ($PSBoundParameters.ContainsKey('ExportEdges')) `
                                'exportEdges' 'settings.exportEdges' $ExportEdges)
    $SaveMinutes = [double](Get-PreviousSetting $previousInstall ($PSBoundParameters.ContainsKey('SaveMinutes')) `
                                'saveMinutes' 'settings.saveMinutes' $SaveMinutes)
    $SaveKeep    = [int](Get-PreviousSetting $previousInstall ($PSBoundParameters.ContainsKey('SaveKeep')) `
                                'saveKeep' 'settings.saveKeep' $SaveKeep)
    $adoptedSaveOnQuit = Get-PreviousSetting $previousInstall ($PSBoundParameters.ContainsKey('SaveOnQuit')) `
                                'saveOnQuit' 'settings.saveOnQuit' $SaveOnQuit
    # The profile spells this as a JSON boolean and the parameter as on|off.
    if ($adoptedSaveOnQuit -is [bool]) {
        $SaveOnQuit = if ($adoptedSaveOnQuit) { 'on' } else { 'off' }
    } elseif ("$adoptedSaveOnQuit" -in @('on', 'off', 'True', 'False')) {
        $SaveOnQuit = if ("$adoptedSaveOnQuit" -in @('on', 'True')) { 'on' } else { 'off' }
    }
    # THE EXCLUSION LIST IS THE ONE WITH TWO SPELLINGS OF "OFF". An empty list is
    # not a value somebody typed - it is what -NoMigrationExclusion leaves behind,
    # and adopting it as a bare empty string would trip the refusal below on an
    # upgrade of an installation that had deliberately turned the policy off.
    if (-not ($PSBoundParameters.ContainsKey('ExcludeSpecies') -or $NoMigrationExclusion)) {
        $adoptedExclude = [string](Get-PreviousSetting $previousInstall $false `
                                        'excludeSpecies' 'settings.excludeSpecies' $ExcludeSpecies)
        if ($adoptedExclude) {
            $ExcludeSpecies = $adoptedExclude
        } elseif ($previousInstall.Profile -or $previousInstall.Record) {
            $NoMigrationExclusion = $true
        }
    }
    Say "keeping the settings this installation already had; name a flag to change one."
}

$edgeList = @($ExportEdges -split '[,; \t]+' | Where-Object { $_ } | ForEach-Object { $_.ToUpperInvariant() })
if ($edgeList.Count -eq 0) {
    Stop-Setup ("-ExportEdges names no edge. Use E, N, W or S, comma separated - normally 'E,N,W,S'. " +
                "If you want this world off the map, do not start it.")
}
foreach ($e in $edgeList) {
    if ($e -notin @('E', 'N', 'W', 'S')) { Stop-Setup "-ExportEdges holds '$e'. Use E, N, W or S." }
}
if (($edgeList | Select-Object -Unique).Count -ne $edgeList.Count) {
    Stop-Setup "-ExportEdges repeats an edge: '$ExportEdges'."
}
$ExportEdges = [string]::Join(',', $edgeList)

if ($NoMigrationExclusion) {
    $ExcludeSpecies = ''
} elseif (-not $ExcludeSpecies) {
    Stop-Setup ("-ExcludeSpecies is empty, which would turn the exclusion policy off. That is a " +
                "real choice and it takes its own switch: pass -NoMigrationExclusion if you mean it.")
}

Write-Host ""
Write-Host "  WHAT A BARE INSTALL DOES, STATED ONCE:" -ForegroundColor Cyan
Write-Host "  YOUR WORLD EXPORTS ON ALL FOUR EDGES. Nothing configured means the whole" -ForegroundColor Cyan
Write-Host "  perimeter, not silence. Every edge is a door that works both ways: organisms" -ForegroundColor Cyan
Write-Host "  leave your world on every side and arrive from every side." -ForegroundColor Cyan
Write-Host ""
Say "This install was set up with export edges: $ExportEdges"
if ($ExportEdges -eq 'E,N,W,S') {
    Say "which is the shipped default. It is written out explicitly all the same, so that"
    Say "a future change to the mod's own default cannot silently move what your world does."
} else {
    Say "which is a subset you asked for. The edges you left out run no capture band; your"
    Say "world still RECEIVES on all four, because an arrival was never gated by this list."
}
Write-Host ""
Say "The settings this install runs with, and what each one spends:"
Write-Host ""
Say "  export edges       $ExportEdges"
if ($ExportEdges -eq 'E,N,W,S') {
    Say "                     your world is a full member of the map in every direction"
} else {
    Say "                     your world exports on those edges only, and receives on all four"
}
if ($ExcludeSpecies) {
    Say "  species that       '$ExcludeSpecies'"
    Say "  never leave        keeps founder stock off a shared map's lanes"
} else {
    Write-Host "       species that       NONE - the exclusion policy is OFF" -ForegroundColor Yellow
    Write-Host "       never leave        you asked for -NoMigrationExclusion. Your world will export" -ForegroundColor Yellow
    Write-Host "                          starter stock onto a shared map, and the map's census shows" -ForegroundColor Yellow
    Write-Host "                          that as entirely normal while it happens." -ForegroundColor Yellow
}
Say "  saves              every $SaveMinutes minutes, keeping $SaveKeep"
Say "                     $SaveKeep copies of your world on your disk. The interval is also how"
Say "                     often your world pauses to write itself out"
Say "  save on quit       $SaveOnQuit"
Say "                     your world is written out when the game closes, so stopping is not losing"
Say "  speed              starts at x$StartupTimeScale"
Say "                     the game's own speed is x1, and the map around you does not run that slow."
Say "                     Your machine is not asked for more than it can draw: the game holds the"
Say "                     speed down to keep the picture smooth. The in-game slider still moves it"
Write-Host ""
Say "Every one of them is set explicitly in $StartName, and you can edit them"
Say "there. The names in that file are the mod's own: MULTIVERSE_EXPORT_EDGES,"
Say "MULTIVERSE_MIGRATION_EXCLUDE, MULTIVERSE_SAVE_MINUTES, MULTIVERSE_SAVE_KEEP,"
Say "MULTIVERSE_SAVE_ON_QUIT and MULTIVERSE_STARTUP_TIME_SCALE."

# ---------------------------------------------------------------- 9. the scripts

Step "9 of 9 - install the launcher, write its profile, $StartName, $StopName and the uninstall's record"

$manageProgramFiles = -not [string]::IsNullOrWhiteSpace($InstallRoot)
if (-not $InstallRoot) { $InstallRoot = $Here }
New-Item -ItemType Directory -Force -Path $InstallRoot | Out-Null
$InstallRoot = (Resolve-Path $InstallRoot).Path

$programFiles = @()
if ($manageProgramFiles) {
    # THE LAUNCHER IS TWO FILES. $LauncherName is the window the shortcuts open,
    # and multiverse-launcher.exe beside it is the same launcher's commands and
    # console menu - the file a script calls, and the file the window itself runs
    # when it is given a command line. A shell does not wait for a process in the
    # Windows GUI subsystem, so one file could not be both. Both are recorded
    # below, so the uninstall removes both.
    #
    # $MatrixName IS INSTALLED, NOT ONLY READ. Step 3 reads it out of the kit to
    # decide whether this game build is supported; the SIDECAR reads it again,
    # every time somebody runs the diagnostic, to say which build they are on -
    # and it looks for it beside its own executable (go/internal/sidecar's
    # supportMatrix). A kit folder is a download that gets deleted, so without a
    # copy here the diagnostic's game-version check answered UNKNOWN on a
    # perfectly ordinary install.
    foreach ($name in @($LauncherName, 'multiverse-launcher.exe', $SidecarName,
                         'Uninstall-BibitesMultiverse.ps1',
                         'public-map.json', $MatrixName, 'README.md', 'LICENSE',
                         'THIRD_PARTY_NOTICES.md', 'bibites-multiverse.ico')) {
        $source = Join-Path $Here $name
        if (-not (Test-Path -LiteralPath $source -PathType Leaf)) {
            Stop-Setup "The package is incomplete: $name is missing." 'INS-CHECKSUM'
        }
        $destination = Join-Path $InstallRoot $name
        $sourceSha = Get-Sha256 $source
        $destinationSha = if (Test-Path -LiteralPath $destination -PathType Leaf) {
            Get-Sha256 $destination
        } else {
            ''
        }
        if ($sourceSha -ne $destinationSha) {
            Copy-ProgramFile $source $destination
        }
        $programFiles += [pscustomobject]@{
            path   = $destination
            sha256 = (Get-Sha256 $destination)
        }
    }

    # WHAT THE RELEASE BEFORE THIS ONE LEFT HERE AND THIS ONE NO LONGER SHIPS.
    # The record is rewritten whole below, so a file the previous list named and
    # this one does not would become unremovable by anything in this project: the
    # uninstall reads the new record, never sees it, and the setup's own
    # `RMDir "$INSTDIR"` is deliberately NOT recursive - so the application folder
    # would survive an uninstall and Windows would go on showing a program that
    # is gone.
    #
    # BY HASH, LIKE EVERY OTHER REMOVAL IN THIS PROJECT. A file whose bytes still
    # match what the previous install left is one nobody has touched since, and
    # is this installer's to take. One that differs is somebody's own and is kept
    # and named, exactly as the uninstall keeps it.
    if ($previousInstall.Record -and
        $previousInstall.Record.PSObject.Properties.Match('program').Count -gt 0 -and
        $previousInstall.Record.program -and
        $previousInstall.Record.program.PSObject.Properties.Match('files').Count -gt 0) {
        $keep = @{}
        foreach ($current in $programFiles) { $keep[$current.path.ToUpperInvariant()] = $true }
        foreach ($old in @($previousInstall.Record.program.files)) {
            $oldPath = Get-JsonField $old 'path'
            $oldSha  = Get-JsonField $old 'sha256'
            if (-not $oldPath -or $keep.ContainsKey($oldPath.ToUpperInvariant())) { continue }
            if (-not (Test-Path -LiteralPath $oldPath -PathType Leaf)) { continue }
            if ($oldSha -and (Get-Sha256 $oldPath) -ne $oldSha) {
                Say "kept: $oldPath is no longer part of this release, but it has CHANGED since the"
                Say "      install that left it, so it is not this setup's to remove."
                continue
            }
            Remove-Item -LiteralPath $oldPath -Force -ErrorAction SilentlyContinue
            if (Test-Path -LiteralPath $oldPath) {
                Say "could not remove $oldPath, which this release no longer ships."
            } else {
                Say "removed $oldPath - the release before this one shipped it and this one does not."
            }
        }
    }
}

$sidecarExe = Join-Path $InstallRoot $SidecarName
$saveOnQuitValue = if ($SaveOnQuit -eq 'on') { 'true' } else { 'false' }

$startBody = @'
# Written by Install-BibitesMultiverse.ps1. Start this world on the map: the
# sidecar first, then the game. The Bibites Multiverse launcher is what this
# world runs from now; this file keeps the values the install was made with.
#
#   .\@@STARTNAME@@             the sidecar, then the game
#   .\@@STARTNAME@@ -GameOnly   the game only, against a sidecar already running
#   .\@@STARTNAME@@ -Headless   the world runs with nothing drawn. The simulation
#                               is unchanged; only the picture is gone
#
# THE ORDER MATTERS. The sidecar mints the local token the game's mod presents,
# into its own data directory, the first time it starts. So the sidecar goes
# first and the game follows.
[CmdletBinding()]
param([switch]$GameOnly, [switch]$Headless)
$ErrorActionPreference = 'Stop'

$GameDir     = '@@GAMEDIR@@'
$DataRoot    = '@@DATAROOT@@'
$RelayUrl    = '@@RELAYURL@@'
$SidecarExe  = '@@SIDECAREXE@@'
$PeerId      = '@@PEERID@@'
$World       = '@@WORLD@@'
$SidecarPort = '@@SIDECARPORT@@'

$dataDir        = Join-Path $DataRoot 'data'
$logDir         = Join-Path $DataRoot 'logs'
$credentialFile = Join-Path $DataRoot 'peer-secret.txt'
$log            = Join-Path $logDir 'sidecar.log'
New-Item -ItemType Directory -Force -Path $dataDir, $logDir | Out-Null

# The mod reads its whole configuration from the environment of the game
# process, which a Windows process inherits from this script.
#
# EVERY ONE OF THESE IS SET EXPLICITLY, INCLUDING THE ONES THAT MATCH THE MOD'S
# OWN DEFAULT. An unset MULTIVERSE_EXPORT_EDGES already means all four edges;
# writing it out anyway is what keeps a future change to that default from
# silently moving what this world does. Edit the values here.
$env:MULTIVERSE_EXPORT_EDGES      = '@@EXPORTEDGES@@'
$env:MULTIVERSE_MIGRATION_EXCLUDE = '@@EXCLUDESPECIES@@'
$env:MULTIVERSE_SAVE_MINUTES      = '@@SAVEMINUTES@@'
$env:MULTIVERSE_SAVE_KEEP         = '@@SAVEKEEP@@'
$env:MULTIVERSE_SAVE_ON_QUIT      = '@@SAVEONQUIT@@'
$env:MULTIVERSE_SIDECAR_PORT      = $SidecarPort
$env:MULTIVERSE_WORLD             = $World
$env:MULTIVERSE_PORTAL            = 'true'
$env:MULTIVERSE_PORTAL_FLOURISHES = 'true'
# The speed your world starts at. The game itself resets every world it loads to
# x1, in code, so without this line your world would start ten times slower than
# the map around it. It is a TARGET: the game holds the applied speed below it
# whenever your machine cannot draw fast enough, which costs simulation speed and
# not smoothness. Drag the speed slider in game to change it for a session, or
# edit this line to change what every start does. 'off' means the game's own x1.
$env:MULTIVERSE_STARTUP_TIME_SCALE = '@@STARTUPTIMESCALE@@'
# The link between the game and the sidecar runs on this machine's loopback and
# is authenticated: the sidecar mints this file at its first start, readable by
# you only, and the game presents its contents on every connection. It is NOT
# the map credential - different secret, different file, different wire - and
# the mod never writes its value to any log.
$env:MULTIVERSE_CONTRACT_A_TOKEN_FILE = Join-Path $dataDir 'contract-a.token'
# The channel this world is ASKED TO SAVE AND QUIT through. A game started with
# -batchmode -nographics has no window, so there is no close request to post to
# it and stopping it any other way loses everything since its last save
# (LOCAL-HEADLESSSTOP). This file needs no window, and it is what makes
# .\@@STOPNAME@@ - and the launcher's own 'stop' - lossless for a headless world.
# The mod reads this variable ONCE, at start, so a world already running without
# it can only be stopped the old way.
$env:MULTIVERSE_CMD_FILE = Join-Path $DataRoot 'cmd.txt'
# A COMMAND LEFT BEHIND MUST NOT QUIT THE WORLD THIS START IS BRINGING UP: the
# mod polls that file every 200 ms from the moment it loads, so a 'quit' an
# interrupted stop left there would be obeyed a second after the game appeared.
Remove-Item -LiteralPath $env:MULTIVERSE_CMD_FILE -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath ($env:MULTIVERSE_CMD_FILE + '.log') -Force -ErrorAction SilentlyContinue

$sidecarPidFile = Join-Path $DataRoot 'sidecar.pid'
$gamePidFile    = Join-Path $DataRoot 'game.pid'

function Get-RecordedProcess {
    param([string]$File)
    if (-not (Test-Path -LiteralPath $File)) { return $null }
    $recordedId = Get-Content -LiteralPath $File | Select-Object -First 1
    if (-not $recordedId) { return $null }
    return Get-Process -Id ([int]$recordedId) -ErrorAction SilentlyContinue
}

$gameLaunch = @{
    FilePath         = (Join-Path $GameDir 'The Bibites.exe')
    WorkingDirectory = $GameDir
    PassThru         = $true
}
if ($Headless) { $gameLaunch['ArgumentList'] = @('-batchmode', '-nographics') }

if ($GameOnly) {
    if (-not (Get-RecordedProcess $sidecarPidFile)) {
        Write-Host "-GameOnly needs the sidecar already running. It is not." -ForegroundColor Red
        Write-Host "Run .\@@STARTNAME@@ with no switch."
        exit 1
    }
    if (Get-RecordedProcess $gamePidFile) {
        Write-Host "The game is already running."
        exit 1
    }
    $game = Start-Process @gameLaunch
    Set-Content -Path $gamePidFile -Value $game.Id -Encoding ASCII
    Write-Host "game started (pid $($game.Id)) against the running sidecar; it loads '$World' by itself."
    Write-Host "The sidecar delivers everything it took custody of while the world was away, paced."
    exit 0
}

if (Get-RecordedProcess $sidecarPidFile) {
    Write-Host "A sidecar is already running. Run .\@@STOPNAME@@ first,"
    Write-Host "or .\@@STARTNAME@@ -GameOnly to start only the game."
    exit 1
}
if (-not (Test-Path $credentialFile)) {
    Write-Host "There is no credential at $credentialFile." -ForegroundColor Red
    Write-Host "Run .\Install-BibitesMultiverse.ps1 again."
    exit 1
}

Remove-Item -Path $log, "$log.out" -ErrorAction SilentlyContinue
# --credential-file carries the SECRET HALF only. The identity half is
# --peer-id, and the relay refuses any connection whose credential does not
# authenticate the identity it claims.
$sidecarArgs = @(
    '--listen',          "127.0.0.1:$SidecarPort",
    '--relay',           $RelayUrl,
    '--peer-id',         $PeerId,
    '--data-dir',        $dataDir,
    '--credential-file', $credentialFile
)
$sidecar = Start-Process -FilePath $SidecarExe -PassThru -WindowStyle Hidden -WorkingDirectory $DataRoot -ArgumentList $sidecarArgs -RedirectStandardError $log -RedirectStandardOutput "$log.out"
Set-Content -Path $sidecarPidFile -Value $sidecar.Id -Encoding ASCII
Write-Host "sidecar started (pid $($sidecar.Id)) -> $RelayUrl"
Write-Host "waiting for the map to grant this world a place ..."

$deadline = (Get-Date).AddSeconds(60)
$granted  = $null
$refused  = $null
while ((Get-Date) -lt $deadline) {
    if (Test-Path $log) {
        $granted = Select-String -Path $log -Pattern 'contract B: slot granted' -SimpleMatch | Select-Object -Last 1
        if ($granted) { break }
        $refused = Select-String -Path $log -Pattern 'placement claim refused', 'HTTP 401', 'certificate did not verify', 'below this relay' |
                   Select-Object -Last 1
        if ($refused) { break }
    }
    if ($sidecar.HasExited) { break }
    Start-Sleep -Milliseconds 500
}

if ($granted) {
    Write-Host ""
    Write-Host "YOU ARE ON THE MAP:" -ForegroundColor Green
    Write-Host "  $($granted.Line)"
} else {
    Write-Host ""
    Write-Host "The map did not grant a place." -ForegroundColor Red
    if ($refused) { Write-Host "  $($refused.Line)" }
    if (Test-Path $log) { Get-Content -Path $log -Tail 20 | ForEach-Object { Write-Host "  $_" } }
    Write-Host ""
    Write-Host "  The five usual causes, in order:"
    Write-Host "   1. The relay is not reachable from here - a name that does not resolve, or a"
    Write-Host "      network that does not carry the connection. Neither is on the map's side."
    Write-Host "   2. 'the relay's TLS certificate did not verify'. On a public map that is a"
    Write-Host "      certificate problem the operator has to fix; on a private map it means the"
    Write-Host "      authority is not trusted here yet - re-run the installer with -CaFile."
    Write-Host "   3. HTTP 401: this credential is not the one the relay holds for $PeerId."
    Write-Host "      Ask the operator for a slot handover. The relay stores only a verifier,"
    Write-Host "      so neither public enrollment nor a join string can recover the secret."
    Write-Host "   4. Your wire version is below the minimum this map admits. Install the newest"
    Write-Host "      release; nobody on the relay's side can push it to you."
    Write-Host "   5. Your game build is not the one this map is on. Only the operator can see"
    Write-Host "      which build that is."
    Write-Host ""
    Write-Host "  Each of those is an entry in docs/error-taxonomy.md, with who has to act."
    Write-Host "  The game was NOT started. Run .\@@STOPNAME@@, then try again."
    exit 1
}

$game = Start-Process @gameLaunch
Set-Content -Path $gamePidFile -Value $game.Id -Encoding ASCII
Write-Host ""
Write-Host "game started (pid $($game.Id)); it loads the world '$World' by itself,"
Write-Host "and seeds it on the first start. It saves itself every @@SAVEMINUTES@@ minutes."
Write-Host "logs: $log  and  $GameDir\BepInEx\LogOutput.log"

# THE GAME STARTING IS NOT THE GAME JOINING. A mod that cannot configure itself
# loads, logs, and then does nothing: no world is loaded, no heartbeat is sent,
# and the game sits at the main menu while the sidecar holds your slot and every
# other part of this script reports success. That is LOCAL-CONFIGRACE, and it was
# silent until this check existed. The sidecar already knows the answer, so ask
# it: /my-slot on its own loopback listener is read-only and needs no token.
$bepInExLog = Join-Path $GameDir 'BepInEx\LogOutput.log'
$modWaitSeconds = @@MODWAITSECONDS@@
$modConnected = $false
Write-Host ""
Write-Host "waiting up to $modWaitSeconds s for the game's mod to reach the sidecar..."
for ($waited = 0; $waited -lt $modWaitSeconds; $waited++) {
    try {
        $view = Invoke-RestMethod -Uri "http://127.0.0.1:$SidecarPort/my-slot" `
                                  -TimeoutSec 2 -UseBasicParsing -ErrorAction Stop
        if ($view.mod.connected) { $modConnected = $true; break }
    } catch {
        # Not listening yet, or too old to serve the path. Either is "not yet".
    }
    Start-Sleep -Seconds 1
}

if ($modConnected) {
    Write-Host "the game joined the map: mod connected, speed x@@STARTUPTIMESCALE@@." -ForegroundColor Green
} else {
    Write-Host ""
    Write-Host "!! THE GAME STARTED BUT ITS MOD HAS NOT REACHED THE SIDECAR after $modWaitSeconds s." -ForegroundColor Yellow
    Write-Host "   Your world is NOT on the map yet: the sidecar holds your slot, and the map shows" -ForegroundColor Yellow
    Write-Host "   it live with no game behind it. Nothing is lost and nothing is broken here." -ForegroundColor Yellow
    Write-Host ""
    Write-Host "   Look in the game's own log:"
    Write-Host "     $bepInExLog"
    Write-Host "   An error line beginning '[M2]' is the settings-file trap, LOCAL-CONFIGRACE. No"
    Write-Host "   'Bibites Multiverse <version> loaded' line at all is LOCAL-STARVATION - the"
    Write-Host "   plugin is not being loaded. Both are in docs/error-taxonomy.md."
    Write-Host ""
    Write-Host "   The remedy for either is to restart this world:"
    Write-Host "     .\@@STOPNAME@@"
    Write-Host "     .\@@STARTNAME@@"
    Write-Host "   If it happens twice in a row, report it with that log and the code above."
    Write-Host ""
    Write-Host "   The world and the sidecar are still running; this is a warning, not a failure."
}

Write-Host ""
Write-Host "Leave both running. Run .\@@STOPNAME@@ when you are done."
'@

$stopBody = @'
# Written by Install-BibitesMultiverse.ps1. Stop this world: the game first,
# then the sidecar.
#
#   .\@@STOPNAME@@             the game and the sidecar
#   .\@@STOPNAME@@ -GameOnly   the game only. The sidecar stays up, keeps this
#                              world's place on the map and keeps taking custody
#                              of everything that arrives while the world is away
#
# Stopping costs nothing and needs nobody. Your place on the map is keyed on
# your world's identity and never expires; your neighbours route around you
# while you are away. See docs/participant/leave.md.
[CmdletBinding()]
param([switch]$GameOnly)
$ErrorActionPreference = 'SilentlyContinue'

$DataRoot = '@@DATAROOT@@'

# Ask before forcing. A close request is what runs the game's own quit path, so
# save-on-quit still happens; the force is the fallback.
#
# FOUR RULES THESE TWO FUNCTIONS KEEP, AND WHY:
#   1. Only a process that is really gone is reported as stopped, and only then
#      is its pid file deleted. The pid file is the ledger the uninstall and the
#      launcher both read; deleting it while the process lives would hide a
#      running world from every tool on this machine. Liveness is decided by
#      Get-Process, never by WaitForExit - a handle this script cannot open can
#      make WaitForExit answer "exited" for a process that is still running.
#   2. A process with no window to close - the sidecar, or a game started with
#      -batchmode -nographics - makes taskkill refuse with a non-zero exit.
#      There is nothing to wait for in that case, so it is forced at once
#      instead of burning the whole timeout.
#   3. THE ASK NEVER CARRIES /T. /T walks the process TREE and refuses the whole
#      call when any member of it needs /F - and the game ALWAYS spawns a
#      windowless UnityCrashHandler64.exe. With /T on the ask, every stop of
#      every world, windowed or not, fell through to the force and skipped
#      save-on-quit. The FORCE carries /T, which is where the tree belongs: that
#      is the case where the crash handler has to go with its parent.
#   4. A game with a mod is asked through the mod FIRST (Request-ModQuit), which
#      is the only ask a headless world can hear.
function Request-ModQuit {
    param([string]$CmdFile)
    if (-not $CmdFile) { return $false }
    $answers = $CmdFile + '.log'
    $token = 'stop-{0}-{1}' -f $PID, [DateTime]::UtcNow.Ticks
    Remove-Item -LiteralPath $CmdFile -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $answers -Force -ErrorAction SilentlyContinue
    # Renamed into place, never written in place: the mod discards a command
    # file that does not end in a newline, and a rename makes it appear whole.
    $temporary = $CmdFile + '.tmp'
    try {
        [IO.File]::WriteAllText($temporary, "$token quit`n", (New-Object Text.UTF8Encoding $false))
        Move-Item -LiteralPath $temporary -Destination $CmdFile -Force
    } catch {
        Remove-Item -LiteralPath $temporary -Force -ErrorAction SilentlyContinue
        return $false
    }
    $taken = $false
    for ($i = 0; $i -lt 5; $i++) {
        if (-not (Test-Path -LiteralPath $CmdFile)) { $taken = $true; break }
        Start-Sleep -Milliseconds 200
    }
    if (-not $taken) {
        # Take the request back: the next start of this world would obey it.
        Remove-Item -LiteralPath $CmdFile -Force -ErrorAction SilentlyContinue
        Write-Host "nothing is reading $CmdFile, so this world's mod cannot be asked to quit."
        Write-Host "It was started before MULTIVERSE_CMD_FILE was set, or its mod is not loaded."
        Write-Host "Start this world again once and the next stop is lossless. Asking the window instead."
        return $false
    }
    for ($i = 0; $i -lt 20; $i++) {
        foreach ($line in @(Get-Content -LiteralPath $answers -ErrorAction SilentlyContinue)) {
            $fields = $line -split '\s+'
            if ($fields.Count -ge 2 -and $fields[0] -eq $token) {
                if ($fields[1] -eq 'OK') {
                    Write-Host "this world's mod took the quit request; it is saving and shutting down."
                    return $true
                }
                Write-Host "this world's mod refused the quit request. Asking the window instead."
                return $false
            }
        }
        Start-Sleep -Milliseconds 200
    }
    Write-Host "this world's mod took the quit request and did not answer it. Asking the window instead."
    return $false
}

function Stop-Recorded {
    param([string]$File, [string]$Name, [int]$WaitSeconds = 30, [string]$CmdFile)
    if (-not (Test-Path $File)) { return }
    $id = (Get-Content -Path $File | Select-Object -First 1)
    $processId = 0
    if (-not [int]::TryParse([string]$id, [ref]$processId)) {
        Write-Host "$File does not name a process id; it is left alone."
        return
    }
    if (-not (Get-Process -Id $processId -ErrorAction SilentlyContinue)) {
        Remove-Item -Path $File -Force
        return
    }
    $asked = $false
    if ($CmdFile) { $asked = Request-ModQuit $CmdFile }
    if (-not $asked) {
        $global:LASTEXITCODE = 1
        & taskkill.exe /PID $processId *> $null
        $asked = ($LASTEXITCODE -eq 0)
    }
    if ($asked) {
        $deadline = (Get-Date).AddSeconds($WaitSeconds)
        while ((Get-Date) -lt $deadline -and
               (Get-Process -Id $processId -ErrorAction SilentlyContinue)) {
            Start-Sleep -Milliseconds 500
        }
    }
    if (Get-Process -Id $processId -ErrorAction SilentlyContinue) {
        & taskkill.exe /PID $processId /T /F *> $null
        Start-Sleep -Milliseconds 500
    }
    if (Get-Process -Id $processId -ErrorAction SilentlyContinue) {
        Stop-Process -Id $processId -Force
        Start-Sleep -Milliseconds 500
    }
    if (Get-Process -Id $processId -ErrorAction SilentlyContinue) {
        Write-Host "COULD NOT STOP $Name (pid $processId). It is still running, so $File is kept."
        return
    }
    Write-Host "stopped $Name (pid $processId)"
    Remove-Item -Path $File -Force
}

Stop-Recorded (Join-Path $DataRoot 'game.pid') 'the game' 30 (Join-Path $DataRoot 'cmd.txt')
Start-Sleep -Seconds 1

if ($GameOnly) {
    Write-Host "the world is down; the sidecar keeps its place and its journal."
    Write-Host "Arrivals accumulate there and are delivered, paced, when the world comes back."
    exit 0
}

Stop-Recorded (Join-Path $DataRoot 'sidecar.pid') 'the sidecar' 10
Start-Sleep -Seconds 1

$left = @(
    (Join-Path $DataRoot 'game.pid')
    (Join-Path $DataRoot 'sidecar.pid')
) | Where-Object { Test-Path -LiteralPath $_ }
Write-Host ("processes still recorded as running: {0} (want 0)" -f $left.Count)
Write-Host "The journal in $DataRoot\data is kept. Do not delete it: it is this machine's"
Write-Host "record of every organism it is holding for somebody."
'@

function Expand-Template {
    param([string]$Body)
    return $Body.Replace('@@GAMEDIR@@',        $GameDir).
                 Replace('@@DATAROOT@@',       $DataRoot).
                 Replace('@@RELAYURL@@',       $RelayUrl).
                 Replace('@@SIDECAREXE@@',     $sidecarExe).
                 Replace('@@PEERID@@',         $peerId).
                 Replace('@@WORLD@@',          $World).
                 Replace('@@EXPORTEDGES@@',    $ExportEdges).
                 Replace('@@EXCLUDESPECIES@@', $ExcludeSpecies).
                 Replace('@@SAVEMINUTES@@',    [string]$SaveMinutes).
                 Replace('@@SAVEKEEP@@',       [string]$SaveKeep).
                 Replace('@@SAVEONQUIT@@',     $saveOnQuitValue).
                 Replace('@@STARTUPTIMESCALE@@', $StartupTimeScale).
                 Replace('@@MODWAITSECONDS@@', [string]$ModWaitSeconds).
                 Replace('@@SIDECARPORT@@',    [string]$SidecarPort).
                 Replace('@@STARTNAME@@',      $StartName).
                 Replace('@@STOPNAME@@',       $StopName)
}

$startPath = Join-Path $InstallRoot $StartName
$stopPath  = Join-Path $InstallRoot $StopName
Set-Content -Path $startPath -Value (Expand-Template $startBody) -Encoding ASCII
Set-Content -Path $stopPath  -Value (Expand-Template $stopBody)  -Encoding ASCII
Say "wrote $startPath"
Say "wrote $stopPath"

# The profile the launcher reads. It states this same world in the launcher's
# own format, so the app entry point and the scripts above describe one world
# rather than two. IT HOLDS NO SECRET: the credential stays in its own file,
# readable by you only, and the launcher never copies it anywhere else.
#
# ON AN UPGRADE THIS FILE IS REWRITTEN, NOT REPLACED. Every setting in it that
# somebody could have changed has already been adopted in step 8, and the three
# below are the ones that have no installer parameter at all:
#
#   headless    the launcher's own switch. Rewriting it as false turned a world
#               a participant runs with no window back into one that opens one,
#               every upgrade.
#   createdUtc  when this world was made. An upgrade is not a creation, and a
#               stamp moved forward makes the launcher's own record of the world
#               younger than the journal beside it.
#   active.txt  which world the launcher opens on. Resetting it to 'default'
#               bounced a participant with several worlds back to the first one.
$profilesDir = Join-Path $InstallRoot $ProfilesDirName
New-Item -ItemType Directory -Force -Path $profilesDir | Out-Null
$defaultProfilePath = Join-Path $profilesDir 'default.json'
$profileHeadless = $false
$profileCreated  = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
if ($previousInstall.Profile) {
    if ($previousInstall.Profile.PSObject.Properties.Match('headless').Count -gt 0) {
        $profileHeadless = [bool]$previousInstall.Profile.headless
    }
    $previousCreated = Get-JsonField $previousInstall.Profile 'createdUtc'
    if ($previousCreated) { $profileCreated = $previousCreated }
}
$defaultProfile = [ordered]@{
    format         = $ProfileFormat
    name           = 'default'
    gameDir        = $GameDir
    dataRoot       = $DataRoot
    sidecarPort    = [int]$SidecarPort
    world          = $World
    headless       = $profileHeadless
    exportEdges    = $ExportEdges
    excludeSpecies = $ExcludeSpecies
    saveMinutes    = [double]$SaveMinutes
    saveKeep       = [int]$SaveKeep
    saveOnQuit     = ($SaveOnQuit -eq 'on')
    peerId         = $peerId
    relayUrl       = $RelayUrl
    createdUtc     = $profileCreated
}
$defaultProfile | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $defaultProfilePath -Encoding ASCII
$activeProfile = if ($previousInstall.Active) { $previousInstall.Active } else { 'default' }
Set-Content -LiteralPath (Join-Path $profilesDir 'active.txt') -Value $activeProfile -Encoding ASCII
Say "wrote $defaultProfilePath - the launcher reads it"
if ($activeProfile -ne 'default') {
    Say "this installation opens on '$activeProfile', which is left as it was."
}

# The record the uninstall reads. It names every path this installer created or
# replaced, with the hash it left behind, so the uninstall can remove exactly
# what was added and leave anything a later hand changed.
$bepInExRecorded = @()
foreach ($rel in $bepInExPaths) {
    $full = Join-Path $GameDir $rel
    if (Test-Path $full) {
        $bepInExRecorded += [pscustomobject]@{ path = $rel; sha256 = (Get-Sha256 $full) }
    }
}

$record = [ordered]@{
    record        = 'bibites-multiverse/install-record/3'
    release       = $Release
    installedUtc  = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    kitDir        = $InstallRoot
    gameDir       = $GameDir
    dataRoot      = $DataRoot
    runtime       = [ordered]@{
        mode                     = $runtimeMode
        managedByThisInstaller   = ($runtimeMode -eq 'bundled')
        root                     = $runtimeRoot
        files                    = $runtimeFiles
    }
    peerId        = $peerId
    relayUrl      = $RelayUrl
    world         = $World
    sidecarPort   = [int]$SidecarPort
    settings      = [ordered]@{
        exportEdges    = $ExportEdges
        excludeSpecies = $ExcludeSpecies
        saveMinutes    = [double]$SaveMinutes
        saveKeep       = [int]$SaveKeep
        saveOnQuit     = $saveOnQuitValue
    }
    plugin        = [ordered]@{
        path            = $pluginDst
        sha256          = $pluginSha
        replacedExisting = $pluginHeld
        configFile      = (Join-Path $GameDir "BepInEx\config\$PluginGuid.cfg")
    }
    bepInEx       = [ordered]@{
        installedByThisInstaller = $bepInExOurs
        version                  = $entry.bepInEx
        files                    = $bepInExRecorded
    }
    certificate   = [ordered]@{
        imported   = $caImported
        thumbprint = $caThumbprint
        storedCopy = $caStored
    }
    generated     = @($startPath, $stopPath)
    program       = [ordered]@{
        root  = $InstallRoot
        files = $programFiles
    }
    profiles      = [ordered]@{
        root    = $profilesDir
        default = 'default'
        file    = $defaultProfilePath
        active  = (Join-Path $profilesDir 'active.txt')
    }
    credential    = $credentialPath
    dataDir       = $dataDir
    logDir        = $logDir
}
$recordPath = Join-Path $DataRoot $RecordName
$record | ConvertTo-Json -Depth 6 | Set-Content -Path $recordPath -Encoding ASCII
Say "wrote $recordPath - the uninstall reads it, and removes only what is named in it"
# The pending record holds a SECOND COPY of a secret. Once the install is
# complete it has no use, on any path that led here, so it does not survive this
# script and cannot be left for an uninstall to keep.
$completedPendingPath = Join-Path $DataRoot 'enrollment-pending.json'
if (Test-Path -LiteralPath $completedPendingPath -PathType Leaf) {
    Remove-Item -LiteralPath $completedPendingPath -Force
    Say 'removed the pending enrollment record after completing the install record'
}

# ---------------------------------------------------------------- done

Write-Host ""
Write-Host "Setup is complete." -ForegroundColor Green
Say ("edition      : {0}" -f $(if ($runtimeMode -eq 'bundled') { 'complete (managed game runtime)' } else { 'add-on (existing game)' }))
Say "map          : $RelayUrl"
Say "your world   : $peerId   world file: $World"
Say "game         : $GameDir"
Say "export edges : $ExportEdges   (all four is the shipped default)"
if ($ExcludeSpecies) { Say "never leaves : $ExcludeSpecies" } else { Say "never leaves : nothing - the exclusion policy is OFF" }
Say "saves        : every $SaveMinutes minutes, keeping $SaveKeep, save on quit $SaveOnQuit"
Say "your files   : $DataRoot"
Say "app launcher : $InstallRoot\$LauncherName   (the window the icons open)"
Say "its commands : $InstallRoot\multiverse-launcher.exe   (what a script calls)"
Say "its profiles : $profilesDir   (this world is 'default')"
if ($caImported) { Say "certificate  : $caThumbprint imported into your own user store" }
else             { Say "certificate  : nothing imported into any trust store" }
if ($existingPeerId -and ($existingPeerId -cne $peerId)) {
    Write-Host ""
    Write-Host "  THIS FOLDER'S WORLD CHANGED IDENTITY. It was $existingPeerId." -ForegroundColor Yellow
    Write-Host "  If a slot handover gave you $peerId, that slot and position came with it and" -ForegroundColor Yellow
    Write-Host "  nothing is stranded. If it did not, $existingPeerId IS NOW DARK ON THE MAP:" -ForegroundColor Yellow
    Write-Host "  its slot stays reserved, with nobody in it, until its operator releases it." -ForegroundColor Yellow
    Write-Host "  That name is the whole of the message they need, and it is kept in" -ForegroundColor Yellow
    Write-Host "  $dataDir\peer-id.previous - docs/participant/leave.md." -ForegroundColor Yellow
}
Write-Host ""
if ($StartAfterInstall) {
    Say 'Next: the installer will connect this world and start the game now.'
} else {
    Say "Next: open the Bibites Multiverse icon, or run  .\$LauncherName  here. It"
    Say "      starts this world, stops it, and can add another world on this machine."
    Say "      For a command line or a script, run  .\multiverse-launcher.exe  instead."
}
Say "Advanced: .\$StartName and .\$StopName still run this world from a console, with"
Say "the values this install was made with. The uninstall script here removes all of it."
Write-Host ""
Say "The four pages written for you, on the release page: install, join, diagnose, leave."

if ($StartAfterInstall) {
    Write-Host ""
    Say 'Starting the sidecar. The game will open after the map grants this world a place.'
    & $startPath
    if (-not $?) { Stop-Setup "The installation succeeded, but the installed world did not start." 'INS-START' }
}
