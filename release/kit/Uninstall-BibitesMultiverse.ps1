<#
.SYNOPSIS
    Remove the Bibites Multiverse mod and its sidecar, and leave the game
    exactly as the installer found it.

.DESCRIPTION
    Runs on Windows PowerShell 5.1 and on PowerShell 7, needs no administrator
    rights, and removes NOTHING it did not put there.

    It reads install-record.json - the record the installer wrote - and works
    from that list alone. Every path in the record was created or replaced by
    the installer, with the hash it left behind, so this script can tell a file
    it installed from a file somebody has since changed. A changed file is
    reported and KEPT.

    WHAT IT REMOVES

      * BepInEx\plugins\BibitesMultiverse.dll, if it is still the file the
        installer put there
      * BepInEx\config\dev.multiverse.bibites.cfg, the plugin's own settings
        file, which nothing else writes
      * BepInEx itself and its generated files - ONLY if the installer put
        BepInEx there. If BepInEx was already installed on this machine, it is
        left completely alone
      * the installed application - BibitesMultiverseLauncher.exe, the launcher's
        window, and multiverse-launcher.exe, its commands, with the sidecar, the
        icon and everything else the installer copied beside them - as long as
        each file is still the one it left there
      * Start-Multiverse.ps1 and Stop-Multiverse.ps1
      * the launcher's profiles directory: one file for every world this
        install added, the name of the world the launcher had selected, and
        each of those worlds' recorded process ids and lock file. A profile
        goes with the installation it describes, with or without
        -RemoveWorldData - the world's own data is not in it and stays where it
        is - and that is what leaves the application directory empty, so the
        step below can take the directory itself away and the setup's own
        uninstaller finds nothing left to trip over
      * the copy of a private map's certificate authority
      * that certificate authority from your own user trust store, if the
        installer imported it and only then
      * an unchanged managed game payload, when this was the complete edition -
        and, once nothing this install recorded is left in
        <data root>\runtimes\<sha>, that whole folder with it, including what
        the game and BepInEx wrote inside it while it ran. It is this package's
        own copy of the game, nothing of yours is in it, and a game folder with
        no game left in it is a folder the next install cannot repair. A game
        file somebody CHANGED is still reported and kept, and keeps the folder
        around it

    WHAT IT WRITES

      * one file: <data root>\logs\uninstall-<utc>.log, every line of this
        ledger as it was printed, so a refusal that says "read the uninstall
        ledger" names something you can still read. With -RemoveWorldData - the
        run that deletes that folder - it goes to %TEMP% instead. A -DryRun
        writes nothing at all, this file included

    WHAT IT KEEPS, DELIBERATELY

      * YOUR WORLDS AND THEIR BACKUPS. They are the game's own files, in the
        game's own folder under %USERPROFILE%\AppData\LocalLow\The Bibites, and
        nothing in this package has ever written outside its own directory
        there. This script does not go near them
      * YOUR JOURNALS AND LOGS - the record of every organism this machine took
        custody of and has not handed on, for every world this install runs.
        Pass -RemoveWorldData to delete them too, and read the warning that
        prints before it happens
      * YOUR MAP CREDENTIAL - peer-secret.txt, one for every world this install
        added. It is the world's, not the install's: the map keeps a verifier
        and can never print that secret again, so removing a build must not end
        the world that ran on it. Installing again over the same data root
        reuses it and spends no second identity. It goes with the journal, under
        -RemoveWorldData, and never on its own
      * any file the installer did not create, including another mod's plugin,
        ANYWHERE OUTSIDE THIS PACKAGE'S OWN MANAGED GAME COPY - a game folder
        you chose is never swept, only the files this install put in it are
        taken, and a game you installed yourself is left exactly as it was found

.PARAMETER DataRoot
    Where this install kept its files. Defaults to
    %LOCALAPPDATA%\BibitesMultiverse.

.PARAMETER RecordFile
    The install record, if it is not in the usual place.

.PARAMETER RemoveWorldData
    Also delete the journal, the logs, the map credential and the data directory
    - of this install and of every other world its launcher added. A journal may
    still hold organisms other worlds handed to this one, and a deleted
    credential ends that world on the map: only a slot handover from the
    operator can give a world a fresh one.

.PARAMETER KeepCertificate
    Leave a private map's certificate authority in your trust store.

.PARAMETER DryRun
    Print the ledger of what would be removed and what would be kept, and change
    nothing.

.EXAMPLE
    .\Uninstall-BibitesMultiverse.ps1 -DryRun

.EXAMPLE
    .\Uninstall-BibitesMultiverse.ps1
#>
[CmdletBinding()]
param(
    [string]$DataRoot = '',
    [string]$RecordFile = '',
    [switch]$RemoveWorldData,
    [switch]$KeepCertificate,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$RecordName = 'install-record.json'
$PluginGuid = 'dev.multiverse.bibites'

# The files a running copy holds open, relative to the folder being checked.
$GameLockProbe    = @('The Bibites.exe', 'BepInEx\plugins\BibitesMultiverse.dll')
$SidecarLockProbe = @('multiverse-sidecar.exe')

# THE LEDGER IS KEPT, NOT ONLY PRINTED. Everything below says what it removed
# and what it left and why - and until this file existed all of it went to a
# console window that the uninstall entry in Installed apps closes the moment the
# script ends. docs/error-taxonomy.md tells a reader to "read the uninstall
# ledger" in more than one remedy, and there was nothing to read. So every line
# this script puts on the screen goes through Write-Screen, which keeps a copy,
# and Save-Ledger writes them out as <data root>\logs\uninstall-<utc>.log - the
# same folder and the same naming the graphical setup's install-<utc>.log uses.
$script:ledgerLines = New-Object System.Collections.ArrayList
$script:ledgerPath  = ''
$script:ledgerSaved = $false

function Write-Screen {
    param([string]$Message = '', [string]$ForegroundColor = '')
    if ($ForegroundColor) { Write-Host $Message -ForegroundColor $ForegroundColor }
    else                  { Write-Host $Message }
    [void]$script:ledgerLines.Add($Message)
}

function Say  { param([string]$m) Write-Screen "     $m" }
function Step { param([string]$m) Write-Screen ""; Write-Screen "==== $m" }

function Save-Ledger {
    # Called once at the end of a run and once from every refusal, because a
    # refusal is the run a reader most needs the words of. A dry run writes no
    # file at all - it changes nothing, and that has to include this.
    if ($script:ledgerSaved) { return }
    if (-not $script:ledgerPath) { return }
    $script:ledgerSaved = $true
    if ($DryRun) {
        Write-Host ""
        Write-Host "     Nothing was written, this ledger included. A real run keeps it in"
        Write-Host "     $script:ledgerPath"
        return
    }
    try {
        Set-Content -LiteralPath $script:ledgerPath -Value $script:ledgerLines -Encoding UTF8 -ErrorAction Stop
        Write-Host ""
        Write-Host "     Every line above is kept in $script:ledgerPath"
    } catch {
        Write-Host ""
        Write-Host "     This ledger could not be written to $script:ledgerPath - $($_.Exception.Message)"
    }
}

function Stop-Uninstall {
    param([string]$m)
    Write-Screen ""
    Write-Screen "STOP: $m" -ForegroundColor Red
    Save-Ledger
    exit 1
}

function Get-Sha256 {
    param([string]$Path)
    return (Get-FileHash -Path $Path -Algorithm SHA256).Hash.ToUpperInvariant()
}

$removed = New-Object System.Collections.ArrayList
$kept    = New-Object System.Collections.ArrayList

function Remove-Recorded {
    param([string]$Path, [string]$Sha256 = '', [string]$What = '')
    if (-not (Test-Path -LiteralPath $Path)) {
        [void]$kept.Add("gone already : $Path")
        return
    }
    if ($Sha256) {
        $got = Get-Sha256 $Path
        if ($got -ne $Sha256.ToUpperInvariant()) {
            [void]$kept.Add("CHANGED since the install, so it is left alone : $Path")
            return
        }
    }
    if (-not $DryRun) { Remove-Item -LiteralPath $Path -Force }
    [void]$removed.Add(("{0}{1}" -f $Path, $(if ($What) { "   ($What)" } else { '' })))
}

# -Pending names paths this run has already claimed for removal. Under -DryRun
# they are still on disk, so without this a dry run would report a directory as
# staying that a real run empties - and would blame files the installer itself
# created for it.
function Remove-EmptyDirectory {
    param([string]$Path, [string[]]$Pending = @())
    if (-not (Test-Path -LiteralPath $Path)) { return }
    $left = @(Get-ChildItem -LiteralPath $Path -Force -ErrorAction SilentlyContinue)
    if ($DryRun -and $Pending.Count -gt 0) {
        $left = @($left | Where-Object { $Pending -notcontains $_.FullName })
    }
    if ($left.Count -gt 0) {
        [void]$kept.Add(("not empty, so it stays : {0} ({1} item(s) this installer did not create)" -f $Path, $left.Count))
        return
    }
    if (-not $DryRun) { Remove-Item -LiteralPath $Path -Force }
    [void]$removed.Add("$Path   (empty directory)")
}

# ---------------------------------------------------------------- the record

Step "the install record"

if (-not $DataRoot) { $DataRoot = Join-Path $env:LOCALAPPDATA 'BibitesMultiverse' }
if (-not $RecordFile) { $RecordFile = Join-Path $DataRoot $RecordName }
if (-not (Test-Path -LiteralPath $RecordFile)) {
    Stop-Uninstall ("No install record at $RecordFile, so this script cannot tell what it put on " +
                    "this machine and refuses to guess. If you moved the data directory, pass " +
                    "-DataRoot. Nothing was removed.")
}
$record = Get-Content -LiteralPath $RecordFile -Raw | ConvertFrom-Json
Say "record   : $RecordFile"
Say "installed: $($record.installedUtc)   release $($record.release)"
Say "game     : $($record.gameDir)"
Say "world id : $($record.peerId)"

# The data root this install recorded for itself, read once. Every question
# about what is inside this install's own folder - which game copy is the
# managed one, which paths a profile may name - is measured against it, because
# the record's dataRoot and the paths beside it were written by one run of the
# installer and cannot disagree with each other.
$recordedDataRoot = ''
if ($record.PSObject.Properties.Match('dataRoot').Count -gt 0) {
    $recordedDataRoot = [string]$record.dataRoot
}

# WHERE THE LEDGER IS KEPT. Beside the world's own logs, because the data root
# survives an ordinary uninstall whole - the journal, the logs and the credential
# are what this script keeps by default, and a reader sent here by a refusal
# still has the folder to look in. The installer's own note about that folder
# holds for this file too: <data root>\logs is not evidence that a world ever
# ran, and a later install does not read this file as one.
#
# -RemoveWorldData DELETES THAT FOLDER, so the ledger goes to %TEMP% instead
# rather than being written into a directory this run is about to remove - or,
# worse, re-created after it, leaving a data root that could not be removed at
# all. Same for a logs directory this account cannot write to.
$ledgerDir = Join-Path $DataRoot 'logs'
if ($RemoveWorldData -and $env:TEMP) { $ledgerDir = $env:TEMP }
try {
    New-Item -ItemType Directory -Force -Path $ledgerDir -ErrorAction Stop | Out-Null
} catch {
    if ($env:TEMP) { $ledgerDir = $env:TEMP } else { $ledgerDir = '' }
}
if ($ledgerDir) {
    $script:ledgerPath = Join-Path $ledgerDir `
        ('uninstall-' + (Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ') + '.log')
    Say "ledger   : $script:ledgerPath"
} else {
    Say "ledger   : nowhere this account can write was found, so this run keeps none"
}

if ($DryRun) {
    Write-Screen ""
    Write-Screen "  -DryRun: nothing below is actually removed." -ForegroundColor Cyan
}

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

# Both checks are on THIS install's own paths rather than on a process name: a
# machine may hold a second copy of the game or a second world, and only the one
# this record describes has to be stopped.
#
# A process whose path this account cannot read - another user's session, or an
# elevated one - is NOT counted on that fact alone. What decides it then is the
# folder itself: -LockProbe names the files a running copy holds open, and if
# none of them is open for writing, nothing is running out of this folder,
# whoever else is running whatever else on this machine.
function Test-ProcessUnder {
    param([string]$Name, [string]$Path, [string[]]$LockProbe = @())
    $opaque = $false
    foreach ($process in @(Get-Process -Name $Name -ErrorAction SilentlyContinue)) {
        $exe = ''
        try { $exe = $process.Path } catch { $exe = '' }
        if (-not $exe) { $opaque = $true; continue }
        if ($exe.StartsWith($Path, [System.StringComparison]::OrdinalIgnoreCase)) { return $true }
    }
    if (-not $opaque) { return $false }
    foreach ($relative in $LockProbe) {
        if (Test-FileLocked (Join-Path $Path $relative)) { return $true }
    }
    return $false
}

# The launcher's own ledger, on disk: one decimal process id on the first line
# of the file, the same convention the generated scripts use. A process id is
# reused by Windows once its process is gone, so the name has to agree too -
# otherwise a stale file can make this script refuse forever, with no way out.
function Test-ProcessRecorded {
    param([string]$File, [string]$ProcessName = '')
    if (-not (Test-Path -LiteralPath $File)) { return $false }
    $recordedId = Get-Content -LiteralPath $File -ErrorAction SilentlyContinue | Select-Object -First 1
    if (-not $recordedId) { return $false }
    $parsed = 0
    if (-not [int]::TryParse([string]$recordedId, [ref]$parsed)) { return $false }
    $process = Get-Process -Id $parsed -ErrorAction SilentlyContinue
    if (-not $process) { return $false }
    if ($ProcessName -and $process.ProcessName -ne $ProcessName) { return $false }
    return $true
}

# A profile file this script cannot read is a file it did not write, or one
# somebody changed. It returns $null for that, and the caller keeps it. The
# format and the name-matches-the-file-name rule are the same two the launcher
# itself enforces when it loads a profile, so the two tools accept exactly the
# same files.
function Get-LauncherProfile {
    param([string]$Path)
    $data = $null
    try { $data = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json } catch { return $null }
    if (-not $data) { return $null }
    foreach ($field in @('format', 'name', 'gameDir', 'dataRoot')) {
        if ($data.PSObject.Properties.Match($field).Count -eq 0) { return $null }
    }
    if ([string]$data.format -ne 'bibites-multiverse/launcher-profile/1') { return $null }
    if ([string]$data.name -ne [System.IO.Path]::GetFileNameWithoutExtension($Path)) { return $null }
    return $data
}

# NOTHING is removed at a path that fails this. A profile file is ordinary
# user-writable JSON in an ordinary user-writable directory, so a hand-edited,
# half-written or foreign file can name any path at all - and -RemoveWorldData
# deletes recursively. A world's data root has to be a rooted path, below a
# drive root, and not this install's game folder, application directory,
# profiles directory or the user's own profile folder - nor a parent of any of
# them. When the answer is "I cannot tell", the file is left alone.
#
# WHAT COUNTS AS "this install's game folder" IS Get-ProtectedRoot'S ANSWER, not
# the record's gameDir read raw: the complete edition's game folder lives INSIDE
# the data root, and protecting a folder this same run reclaims made every
# complete-edition data root fail this check and left the profiles - and the
# application directory holding them - behind for ever.
function Test-SafeDataRoot {
    param([string]$Path, [string[]]$Protected = @())
    if (-not $Path) { return $false }
    if (-not [System.IO.Path]::IsPathRooted($Path)) { return $false }
    $full = ''
    try { $full = [System.IO.Path]::GetFullPath($Path) } catch { return $false }
    if (-not [System.IO.Path]::GetDirectoryName($full)) { return $false }
    $me = $full.TrimEnd('\', '/')
    if (-not $me) { return $false }
    foreach ($other in $Protected) {
        if (-not $other) { continue }
        $target = ''
        try { $target = [System.IO.Path]::GetFullPath([string]$other) } catch { continue }
        $target = $target.TrimEnd('\', '/')
        if (-not $target) { continue }
        if ($me -eq $target) { return $false }
        # The platform's own separator rather than a literal backslash: both
        # sides have been through GetFullPath, which on Windows normalises a
        # forward slash to a backslash, so this is the same comparison it always
        # made - and it is one release/test-runtime-rules.ps1 can drive from a
        # machine that is not Windows.
        if ($target.StartsWith(($me + [System.IO.Path]::DirectorySeparatorChar),
                               [System.StringComparison]::OrdinalIgnoreCase)) { return $false }
    }
    return $true
}

function Get-LauncherProfileFile {
    param([string]$Root)
    if (-not $Root -or -not (Test-Path -LiteralPath $Root)) { return @() }
    return @(Get-ChildItem -LiteralPath $Root -Filter '*.json' -File -Force -ErrorAction SilentlyContinue |
             Sort-Object Name | ForEach-Object { $_.FullName })
}

function Test-ManagedRuntimePath {
    # THE ONE FOLDER THIS SCRIPT MAY REMOVE WHOLE, and the answer is no for every
    # other path on the machine. It is the same rule, written the same way, as
    # the installer's own Test-ManagedRuntimePath - the two scripts have to agree
    # about which folder the complete edition owns, and a rule in one place that
    # the other only approximates is how they would come to disagree.
    #
    # PROVEN MEANS ALL OF THIS, and any one of them failing is a refusal:
    #
    #   * no `..` anywhere in the path as written, refused before normalisation
    #     can turn it back into a folder that looks legitimate
    #   * its parent is EXACTLY <data root>\runtimes - not a folder below it, not
    #     the data root itself, not a sibling whose name starts the same way
    #   * its own name is a 64-character hex SHA-256, which is how the installer
    #     names a managed runtime and nothing else in that folder is called
    #
    # The record's own dataRoot is what it is measured against, because the
    # record's runtime.root and its dataRoot were written by one run of the
    # installer and cannot disagree with each other.
    param([string]$Path, [string]$DataRoot)
    if (-not $Path -or -not $DataRoot) { return $false }
    foreach ($segment in ($Path -split '[\\/]+')) {
        if ($segment -eq '..') { return $false }
    }
    $full = ''
    $runtimes = ''
    try {
        $full = [System.IO.Path]::GetFullPath($Path)
        $runtimes = [System.IO.Path]::GetFullPath((Join-Path ([System.IO.Path]::GetFullPath($DataRoot)) 'runtimes'))
    } catch {
        return $false
    }
    $me = $full.TrimEnd('\', '/')
    if (-not $me) { return $false }
    $parent = ''
    try { $parent = [System.IO.Path]::GetDirectoryName($me) } catch { return $false }
    if (-not $parent) { return $false }
    # Windows file names are case-insensitive and so is this, deliberately:
    # RUNTIMES\ is the same folder as runtimes\ and has to be as safe.
    if ($parent.TrimEnd('\', '/') -ne $runtimes.TrimEnd('\', '/')) { return $false }
    if ([System.IO.Path]::GetFileName($me) -notmatch '^[0-9A-Fa-f]{64}$') { return $false }
    return $true
}

function Get-ProtectedRoot {
    # THE PATHS NO PROFILE MAY NAME AS A WORLD'S DATA ROOT - minus the one this
    # uninstall removes itself.
    #
    # Test-SafeDataRoot refuses a data root that CONTAINS a protected path, and
    # the game folder is protected: a profile claiming the folder that holds
    # somebody's game as its data root would have this script deleting inside it.
    # THE COMPLETE EDITION'S GAME FOLDER IS INSIDE THE DATA ROOT BY DESIGN -
    # <data root>\runtimes\<sha> - so that rule made every complete-edition data
    # root "unsafe", against a folder this very run has just reclaimed. What it
    # cost was never the world's data, which is kept for its own reasons and was
    # not at risk: it was the profiles. Every profile was left in place "because
    # its data root is not a path this script will act on", profiles\ was then
    # not empty, the application folder holding it was not empty either, and the
    # setup's own uninstaller could not take its directory away - so an install
    # that had been removed still had a folder, a profile and an entry, all
    # describing a world that was no longer installed.
    #
    # A MANAGED RUNTIME IS THEREFORE NOT PROTECTED FROM THIS SCRIPT, because this
    # script is what removes it. Nothing else is dropped: the application folder,
    # the profiles folder, the kit and %USERPROFILE% still refuse, and a game
    # folder somebody CHOSE - an add-on install's Steam copy, or anything else
    # Test-ManagedRuntimePath does not accept - is protected exactly as before.
    param([string[]]$Paths, [string]$DataRoot)
    $out = New-Object System.Collections.ArrayList
    foreach ($path in $Paths) {
        if (-not $path) { continue }
        if (Test-ManagedRuntimePath ([string]$path) $DataRoot) { continue }
        [void]$out.Add([string]$path)
    }
    return @($out.ToArray())
}

if (Test-ProcessUnder 'The Bibites' $record.gameDir $GameLockProbe) {
    Stop-Uninstall ("The Bibites is running from $($record.gameDir). Close it first; Windows holds " +
                    "the plugin open while the game runs. Nothing was removed.")
}
if (Test-ProcessUnder 'multiverse-sidecar' $record.kitDir $SidecarLockProbe) {
    Stop-Uninstall "This install's sidecar is running. Run .\Stop-Multiverse.ps1 first. Nothing was removed."
}

# Every other world this install's launcher added has its own data root, its own
# sidecar and its own game process, and none of them is under the two paths
# above. They are checked from the launcher's profiles.
$profilesRoot = ''
if ($record.PSObject.Properties.Match('profiles').Count -gt 0 -and
    $record.profiles.PSObject.Properties.Match('root').Count -gt 0) {
    $profilesRoot = [string]$record.profiles.root
}
$profileFiles = @(Get-LauncherProfileFile $profilesRoot)

# The paths no profile may ever name as its data root - this install's own game
# folder among them, unless that folder is the managed game copy this run
# reclaims itself. Get-ProtectedRoot has the whole of that reasoning.
$protectedRoots = @(Get-ProtectedRoot @([string]$record.gameDir, [string]$record.kitDir,
                                        $profilesRoot, $env:USERPROFILE) $recordedDataRoot)
if ($record.PSObject.Properties.Match('program').Count -gt 0 -and
    $record.program.PSObject.Properties.Match('root').Count -gt 0) {
    $protectedRoots += [string]$record.program.root
}

# The pid file each world keeps, and the process name that has to be running for
# the record to mean anything.
$profilePidFiles = [ordered]@{ 'sidecar.pid' = 'multiverse-sidecar'; 'game.pid' = 'The Bibites' }

$profileGameDirs = @()
foreach ($profilePath in $profileFiles) {
    $profileData = Get-LauncherProfile $profilePath
    if (-not $profileData) { continue }
    $profileRoot = [string]$profileData.dataRoot
    if (-not (Test-SafeDataRoot $profileRoot (@($protectedRoots) + @(Get-ProtectedRoot @([string]$profileData.gameDir) $recordedDataRoot)))) { continue }
    foreach ($pidFileName in $profilePidFiles.Keys) {
        $pidFilePath = Join-Path $profileRoot $pidFileName
        if (Test-ProcessRecorded $pidFilePath $profilePidFiles[$pidFileName]) {
            Stop-Uninstall ("The world '$($profileData.name)' is still running: $pidFilePath names a " +
                            "live $($profilePidFiles[$pidFileName]). Stop every world first - open " +
                            "Bibites Multiverse and choose Stop, or run " +
                            "multiverse-launcher.exe stop --all. If that world is not really " +
                            "running, delete $pidFilePath and run this again. Nothing was removed.")
        }
    }
    $profileGame = [string]$profileData.gameDir
    if ($profileGame -and ($profileGameDirs -notcontains $profileGame)) {
        $profileGameDirs += $profileGame
    }
}
foreach ($profileGame in $profileGameDirs) {
    if (Test-ProcessUnder 'The Bibites' $profileGame $GameLockProbe) {
        Stop-Uninstall ("The Bibites is running from $profileGame, which one of this install's " +
                        "worlds uses. Close it first. Nothing was removed.")
    }
}

# ---------------------------------------------------------------- the plugin

Step "the plugin"

Remove-Recorded -Path $record.plugin.path -Sha256 $record.plugin.sha256 -What 'the mod'
Remove-Recorded -Path $record.plugin.configFile -What "the plugin's own settings, written by BepInEx"

# ---------------------------------------------------------------- BepInEx

Step "BepInEx"

if (-not $record.bepInEx.installedByThisInstaller) {
    Say "BepInEx was already on this machine before the install, so none of it is touched."
    [void]$kept.Add("BepInEx : already present before the install; left whole")
} else {
    $gameDir = $record.gameDir
    foreach ($file in @($record.bepInEx.files)) {
        Remove-Recorded -Path (Join-Path $gameDir $file.path) -Sha256 $file.sha256 -What 'BepInEx'
    }

    # What BepInEx itself writes once the game has run: its own configuration,
    # its log and its cache. They exist because this installer put BepInEx here,
    # so they go with it - but only after every foreign file has been counted.
    $bepInExDir = Join-Path $gameDir 'BepInEx'
    foreach ($generated in @('LogOutput.log', 'LogOutput.log.1', 'LogOutput.log.2',
                             'LogOutput.log.3', 'LogOutput.log.4',
                             'config\BepInEx.cfg')) {
        Remove-Recorded -Path (Join-Path $bepInExDir $generated) -What 'written by BepInEx after it was installed'
    }
    $cacheDir = Join-Path $bepInExDir 'cache'
    if (Test-Path -LiteralPath $cacheDir) {
        foreach ($cached in @(Get-ChildItem -LiteralPath $cacheDir -File -Force -ErrorAction SilentlyContinue)) {
            Remove-Recorded -Path $cached.FullName -What "BepInEx's cache"
        }
    }

    # Directories last, deepest first, and only while they are empty. Anything
    # left inside one is a file this installer did not create - another mod's
    # plugin, a log somebody kept - and it keeps its directory.
    $dirs = @()
    if (Test-Path -LiteralPath $bepInExDir) {
        $dirs = @(Get-ChildItem -LiteralPath $bepInExDir -Directory -Recurse -Force -ErrorAction SilentlyContinue |
                  Sort-Object { $_.FullName.Length } -Descending | ForEach-Object { $_.FullName })
    }
    foreach ($dir in $dirs) { Remove-EmptyDirectory $dir }
    Remove-EmptyDirectory $bepInExDir
}

# ---------------------------------------------------------------- the managed runtime

Step "the managed game runtime"

$hasRuntime = $record.PSObject.Properties.Match('runtime').Count -gt 0
if (-not $hasRuntime -or -not $record.runtime.managedByThisInstaller) {
    Say "This was the add-on edition, so the game installation is not this package's to remove."
    [void]$kept.Add('game runtime : external installation; left whole')
} else {
    foreach ($file in @($record.runtime.files)) {
        Remove-Recorded -Path $file.path -Sha256 $file.sha256 -What 'the complete edition game payload'
    }

    $runtimeRoot = [string]$record.runtime.root

    # DID ANY FILE THIS INSTALL PUT THERE SURVIVE? That is what decides whether
    # the folder is still somebody's game copy or only what is left of one.
    #
    # It is asked of the hashes rather than of the ledger, and -DryRun is why:
    # under a dry run every recorded file is still on disk, so "it is there"
    # means nothing. A recorded file that still MATCHES its recorded hash is one
    # this run removes (or would); a recorded file that is there and DIFFERS is
    # one the ledger has just reported CHANGED and kept, and that is a survivor.
    $runtimeSurvivors = @()
    foreach ($file in @($record.runtime.files)) {
        if (-not (Test-Path -LiteralPath $file.path -PathType Leaf)) { continue }
        if ((Get-Sha256 $file.path) -eq ([string]$file.sha256).ToUpperInvariant()) { continue }
        $runtimeSurvivors += [string]$file.path
    }

    # What is in the folder that this install never recorded: BepInEx's log, its
    # configuration and its cache, whatever else the game wrote while it ran.
    $recordedRuntimePaths = @{}
    foreach ($file in @($record.runtime.files)) {
        $recordedRuntimePaths[([string]$file.path).ToUpperInvariant()] = $true
    }
    $runtimeResidue = @()
    if ($runtimeRoot -and (Test-Path -LiteralPath $runtimeRoot -PathType Container)) {
        $runtimeResidue = @(Get-ChildItem -LiteralPath $runtimeRoot -File -Force -Recurse -ErrorAction SilentlyContinue |
                            Where-Object { -not $recordedRuntimePaths.ContainsKey($_.FullName.ToUpperInvariant()) })
    }

    if ($runtimeRoot -and (Test-Path -LiteralPath $runtimeRoot -PathType Container)) {
        if ($runtimeSurvivors.Count -eq 0 -and $runtimeResidue.Count -gt 0 -and
            (Test-ManagedRuntimePath $runtimeRoot $recordedDataRoot)) {
            # THE MANAGED GAME COPY IS RECLAIMED WHOLE, RESIDUE AND ALL. Not one
            # file this install recorded is left in <data root>\runtimes\<sha>,
            # so what is in it is no longer a game: it is BepInEx's log, its
            # configuration, its cache, and whatever else ran there. Leaving that
            # behind is what broke the next install - it found a folder with no
            # game in it, refused to overwrite it, and named a file nobody could
            # put back. This folder is this package's own and holds nothing of
            # yours; your worlds are the game's own saves elsewhere, and the
            # journal, the logs and the credential are OUTSIDE it in the data
            # root, which this script keeps unless you ask for -RemoveWorldData.
            #
            # A SURVIVING RECORDED FILE STOPS THIS. A game file somebody changed
            # is reported CHANGED and kept above, and then the folder around it
            # keeps itself too: this sweep is for rubble, never for a folder
            # something in it is still vouching for.
            Say ("nothing this install recorded is left in {0}, and {1} file(s) the game and" -f `
                 $runtimeRoot, $runtimeResidue.Count)
            Say "BepInEx wrote while it ran are. That folder is this package's own copy of the game,"
            Say "so it goes whole - a game folder with no game in it serves nobody and is exactly"
            Say "what an install over it cannot repair."
            if (-not $DryRun) { Remove-Item -LiteralPath $runtimeRoot -Recurse -Force }
            [void]$removed.Add(
                (("{0}   ({1} file(s) created by the game and BepInEx after the install, " +
                  "and the managed game copy around them)") -f $runtimeRoot, $runtimeResidue.Count))
            Remove-EmptyDirectory (Split-Path -Parent $runtimeRoot) -Pending @($runtimeRoot)
        } else {
            # Directories go only when empty. A changed game file or any file
            # somebody added keeps itself and every parent directory it needs.
            $runtimeDirs = @(Get-ChildItem -LiteralPath $runtimeRoot -Directory -Recurse -Force -ErrorAction SilentlyContinue |
                             Sort-Object { $_.FullName.Length } -Descending | ForEach-Object { $_.FullName })
            foreach ($dir in $runtimeDirs) { Remove-EmptyDirectory $dir }
            Remove-EmptyDirectory $runtimeRoot
            Remove-EmptyDirectory (Split-Path -Parent $runtimeRoot)
        }
    }
}

# ---------------------------------------------------------------- the certificate

Step "the certificate authority"

if (-not $record.certificate.imported) {
    Say "The installer imported nothing into any trust store, so there is nothing to take out."
    Say "A public map's relay uses an authority Windows already trusts, and that one is not"
    Say "this package's to add or remove."
} elseif ($KeepCertificate) {
    Say "-KeepCertificate was given; $($record.certificate.thumbprint) stays in Cert:\CurrentUser\Root."
    [void]$kept.Add("certificate : $($record.certificate.thumbprint) left in your user trust store by request")
} else {
    $thumb = $record.certificate.thumbprint
    $found = @(Get-ChildItem 'Cert:\CurrentUser\Root' -ErrorAction SilentlyContinue |
               Where-Object { $_.Thumbprint -eq $thumb })
    if ($found.Count -eq 0) {
        [void]$kept.Add("certificate : $thumb is no longer in your user trust store")
    } else {
        if (-not $DryRun) { $found | Remove-Item }
        [void]$removed.Add("Cert:\CurrentUser\Root\$thumb   (the private map's certificate authority)")
    }
}

# ---------------------------------------------------------------- the profiles

# Before this install's own files, because the profiles directory sits inside
# the installed application directory, and the step below only takes that
# directory away once it is empty.
Step "the profiles"

if (-not $profilesRoot) {
    Say "this record names no launcher profiles, so there are none to remove."
} elseif (-not (Test-Path -LiteralPath $profilesRoot)) {
    [void]$kept.Add("gone already : $profilesRoot")
} else {
    # Only a world with its own data root has anything here to remove: the first
    # world's files are this install's own, removed from the record below. The
    # warning is for the worlds beyond that one, so it is counted first rather
    # than printed on every single-world uninstall.
    $extraWorlds = 0
    foreach ($profilePath in $profileFiles) {
        $countData = Get-LauncherProfile $profilePath
        if (-not $countData) { continue }
        $countRoot = [string]$countData.dataRoot
        if ($countRoot -eq [string]$record.dataRoot) { continue }
        if (-not (Test-SafeDataRoot $countRoot (@($protectedRoots) + @(Get-ProtectedRoot @([string]$countData.gameDir) $recordedDataRoot)))) { continue }
        $extraWorlds++
    }
    if ($RemoveWorldData -and $extraWorlds -gt 0) {
        Write-Screen ""
        Write-Screen "  -RemoveWorldData: EVERY world's journal and credential goes, not only the first one's." -ForegroundColor Yellow
        Write-Screen "  Each journal is this machine's record of the organisms that world took custody" -ForegroundColor Yellow
        Write-Screen "  of and has not handed on. Nobody else holds a copy. Your worlds are NOT in it." -ForegroundColor Yellow
        Write-Screen ("  Worlds beyond the first with a journal of their own: {0}." -f $extraWorlds) -ForegroundColor Yellow
        Write-Screen ""
    }
    $profilesPending = New-Object System.Collections.ArrayList
    foreach ($profilePath in $profileFiles) {
        $profileData = Get-LauncherProfile $profilePath
        if (-not $profileData) {
            [void]$kept.Add("not a profile this installer wrote, so it stays : $profilePath")
            continue
        }
        $profileName = [string]$profileData.name
        $profileRoot = [string]$profileData.dataRoot
        if (-not (Test-SafeDataRoot $profileRoot (@($protectedRoots) + @(Get-ProtectedRoot @([string]$profileData.gameDir) $recordedDataRoot)))) {
            [void]$kept.Add(("its data root is not a path this script will act on, so the whole " +
                             "profile stays : {0} (dataRoot '{1}')" -f $profilePath, $profileRoot))
            continue
        }
        # The first world's own files are removed with this install's own files
        # below, from the record rather than from the profile.
        if ($profileRoot -ne [string]$record.dataRoot) {
            $worldPending = New-Object System.Collections.ArrayList
            $worldLeftovers = @('sidecar.pid', 'game.pid', 'launcher.lock')
            if ($RemoveWorldData) { $worldLeftovers = @('peer-secret.txt', 'enrollment-pending.json') + $worldLeftovers }
            foreach ($leftover in $worldLeftovers) {
                Remove-Recorded -Path (Join-Path $profileRoot $leftover) -What "the world '$profileName'"
                [void]$worldPending.Add((Join-Path $profileRoot $leftover))
            }
            if ((-not $RemoveWorldData) -and (Test-Path -LiteralPath (Join-Path $profileRoot 'peer-secret.txt'))) {
                [void]$kept.Add("identity: $(Join-Path $profileRoot 'peer-secret.txt') - the world '$profileName' keeps its place on the map")
            }
            if ($RemoveWorldData) {
                foreach ($dir in @((Join-Path $profileRoot 'data'), (Join-Path $profileRoot 'logs'))) {
                    if (Test-Path -LiteralPath $dir) {
                        if (-not $DryRun) { Remove-Item -LiteralPath $dir -Recurse -Force }
                        [void]$removed.Add("$dir   (recursively, by request)")
                    }
                    [void]$worldPending.Add($dir)
                }
                Remove-EmptyDirectory $profileRoot -Pending @($worldPending)
            } else {
                [void]$kept.Add("journal : $(Join-Path $profileRoot 'data') - custody of organisms the world '$profileName' still holds")
                [void]$kept.Add("logs    : $(Join-Path $profileRoot 'logs')")
            }
        }
        # A profile is not hash-guarded the way a program file is: the launcher
        # rewrites it every time somebody edits a world, so no recorded hash
        # could survive. It is removed because this install wrote it.
        Remove-Recorded -Path $profilePath -What "the launcher's profile for '$profileName'"
        [void]$profilesPending.Add($profilePath)
    }
    $activeProfilePath = Join-Path $profilesRoot 'active.txt'
    Remove-Recorded -Path $activeProfilePath -What 'the world the launcher had selected'
    [void]$profilesPending.Add($activeProfilePath)
    Remove-EmptyDirectory $profilesRoot -Pending @($profilesPending)
}

# ---------------------------------------------------------------- this install's own files

Step "this install's own files"

foreach ($script in @($record.generated)) {
    Remove-Recorded -Path $script -What 'written by the installer'
}
if ($record.PSObject.Properties.Match('program').Count -gt 0) {
    foreach ($file in @($record.program.files)) {
        Remove-Recorded -Path $file.path -Sha256 $file.sha256 -What 'the installed application'
    }
}
$pendingCredentialPath = Join-Path $record.dataRoot 'enrollment-pending.json'
if ($RemoveWorldData) {
    Remove-Recorded -Path $record.credential -What 'your map credential, with the world data'
    if (Test-Path -LiteralPath $pendingCredentialPath) {
        Remove-Recorded -Path $pendingCredentialPath -What 'a half-finished enrollment, with the world data'
    }
} else {
    # THE CREDENTIAL IS THE WORLD'S, NOT THE INSTALL'S. The map keeps a verifier
    # and cannot print the secret again, so deleting it here would end the world
    # on the map for a person who is only replacing a build - and installing
    # again over this data root reuses it rather than spending another identity.
    # It goes with the journal, under -RemoveWorldData, and never on its own.
    [void]$kept.Add("identity: $($record.credential) - the world $($record.peerId) keeps its place on the map, and installing again here reuses it")
    if (Test-Path -LiteralPath $pendingCredentialPath) {
        # It holds a second copy of a secret, so it is never kept silently.
        [void]$kept.Add("pending : $pendingCredentialPath - a half-finished enrollment, and a second copy of a secret. Installing again finishes it; -RemoveWorldData removes it")
    }
}
if ($record.certificate.storedCopy) {
    Remove-Recorded -Path $record.certificate.storedCopy -What "the copy of the map's certificate authority"
}
foreach ($leftover in @('sidecar.pid', 'game.pid')) {
    Remove-Recorded -Path (Join-Path $record.dataRoot $leftover)
}

if ($RemoveWorldData) {
    Write-Screen ""
    Write-Screen "  -RemoveWorldData: the journal and this world's identity go too." -ForegroundColor Yellow
    Write-Screen "  The journal is this machine's record of every organism it took custody of and" -ForegroundColor Yellow
    Write-Screen "  has not handed on. Nobody else holds a copy, and no operator command can reach" -ForegroundColor Yellow
    Write-Screen "  it. Deleting it drops whatever it still held. Your worlds are NOT in it and are" -ForegroundColor Yellow
    Write-Screen "  not affected." -ForegroundColor Yellow
    Write-Screen "  The credential goes with it, and that is the end of this world on the map: the" -ForegroundColor Yellow
    Write-Screen "  relay keeps a verifier and cannot print that secret again. Its slot stays" -ForegroundColor Yellow
    Write-Screen "  reserved until you ask the operator to release it or hand it on." -ForegroundColor Yellow
    Write-Screen ""
    foreach ($dir in @($record.dataDir, $record.logDir)) {
        if (Test-Path -LiteralPath $dir) {
            if (-not $DryRun) { Remove-Item -LiteralPath $dir -Recurse -Force }
            [void]$removed.Add("$dir   (recursively, by request)")
        }
    }
} else {
    [void]$kept.Add("journal : $($record.dataDir) - custody of organisms this world still holds")
    [void]$kept.Add("logs    : $($record.logDir)")
}

if (-not $DryRun) { Remove-Item -LiteralPath $RecordFile -Force }
[void]$removed.Add("$RecordFile   (the install record itself, last)")
if ($record.PSObject.Properties.Match('program').Count -gt 0) {
    Remove-EmptyDirectory ([string]$record.program.root)
}
if (-not $RemoveWorldData) {
    Say "the journal, the logs and this world's identity stay. Installing again over"
    Say "$($record.dataRoot) reuses that identity. Pass -RemoveWorldData to remove them as well."
} else {
    Remove-EmptyDirectory $record.dataRoot
}

# ---------------------------------------------------------------- the ledger

Write-Screen ""
Write-Screen "==== what was removed" -ForegroundColor Green
foreach ($item in $removed) { Say $item }
Write-Screen ""
Write-Screen "==== what was kept, and why"
foreach ($item in $kept) { Say $item }
Write-Screen ""
Say "Untouched, in every run of this script: your worlds and their backups, in the"
Say "game's own folder under %USERPROFILE%\AppData\LocalLow\The Bibites. This package"
Say "never wrote outside its own directory there, and this script never reads them."
Write-Screen ""
if ($DryRun) {
    Write-Screen "Nothing was changed. Run it again without -DryRun to do it." -ForegroundColor Cyan
} else {
    if ($hasRuntime -and $record.runtime.managedByThisInstaller) {
        Write-Screen "Done. Every unchanged managed game file was removed." -ForegroundColor Green
        Say "A changed or user-added runtime file remains in place, if the ledger above names one,"
        Say "and keeps the folder around it. With nothing of this install's left in that folder, the"
        Say "managed game copy goes whole - what the game wrote inside it included."
    } else {
        Write-Screen "Done. The game is as the installer found it." -ForegroundColor Green
        Say "Steam can verify that for you: Properties -> Installed Files -> Verify integrity."
    }
}
Write-Screen ""
Say "Leaving a map is a separate act from uninstalling, and it is one message to the"
Say "map's operator. Until they release your place, the map keeps it for you and every"
Say "organism addressed to it waits out its hold. See docs/participant/leave.md."

Save-Ledger
