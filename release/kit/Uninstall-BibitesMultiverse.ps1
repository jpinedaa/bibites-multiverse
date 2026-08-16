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
      * the installed application - BibitesMultiverseLauncher.exe, the sidecar,
        the icon and everything else the installer copied beside them - as long
        as each file is still the one it left there
      * Start-Multiverse.ps1 and Stop-Multiverse.ps1
      * the launcher's profiles directory: one file for every world this
        install added, the name of the world the launcher had selected, and
        each of those worlds' recorded process ids and lock file
      * your map credential - one for every world this install added - and the
        copy of a private map's certificate authority
      * that certificate authority from your own user trust store, if the
        installer imported it and only then
      * an unchanged managed game payload, when this was the complete edition;
        changed and user-added files are kept

    WHAT IT KEEPS, DELIBERATELY

      * YOUR WORLDS AND THEIR BACKUPS. They are the game's own files, in the
        game's own folder under %USERPROFILE%\AppData\LocalLow\The Bibites, and
        nothing in this package has ever written outside its own directory
        there. This script does not go near them
      * YOUR JOURNALS AND LOGS - the record of every organism this machine took
        custody of and has not handed on, for every world this install runs.
        Pass -RemoveWorldData to delete them too, and read the warning that
        prints before it happens
      * any file the installer did not create, including another mod's plugin

.PARAMETER DataRoot
    Where this install kept its files. Defaults to
    %LOCALAPPDATA%\BibitesMultiverse.

.PARAMETER RecordFile
    The install record, if it is not in the usual place.

.PARAMETER RemoveWorldData
    Also delete the journal, the logs and the data directory - of this install
    and of every other world its launcher added. A journal may still hold
    organisms other worlds handed to this one.

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

function Say  { param([string]$m) Write-Host "     $m" }
function Step { param([string]$m) Write-Host ""; Write-Host "==== $m" }
function Stop-Uninstall {
    param([string]$m)
    Write-Host ""
    Write-Host "STOP: $m" -ForegroundColor Red
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
if ($DryRun) {
    Write-Host ""
    Write-Host "  -DryRun: nothing below is actually removed." -ForegroundColor Cyan
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
        if ($target.StartsWith(($me + '\'), [System.StringComparison]::OrdinalIgnoreCase)) { return $false }
    }
    return $true
}

function Get-LauncherProfileFile {
    param([string]$Root)
    if (-not $Root -or -not (Test-Path -LiteralPath $Root)) { return @() }
    return @(Get-ChildItem -LiteralPath $Root -Filter '*.json' -File -Force -ErrorAction SilentlyContinue |
             Sort-Object Name | ForEach-Object { $_.FullName })
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

# The paths no profile may ever name as its data root.
$protectedRoots = @([string]$record.gameDir, [string]$record.kitDir, $profilesRoot, $env:USERPROFILE)
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
    if (-not (Test-SafeDataRoot $profileRoot (@($protectedRoots) + @([string]$profileData.gameDir)))) { continue }
    foreach ($pidFileName in $profilePidFiles.Keys) {
        $pidFilePath = Join-Path $profileRoot $pidFileName
        if (Test-ProcessRecorded $pidFilePath $profilePidFiles[$pidFileName]) {
            Stop-Uninstall ("The world '$($profileData.name)' is still running: $pidFilePath names a " +
                            "live $($profilePidFiles[$pidFileName]). Stop every world first - open " +
                            "Bibites Multiverse and choose Stop, or run " +
                            "BibitesMultiverseLauncher.exe stop --all. If that world is not really " +
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

    # Directories go only when empty. A changed game file or any file somebody
    # added keeps itself and every parent directory it needs.
    $runtimeRoot = [string]$record.runtime.root
    if ($runtimeRoot -and (Test-Path -LiteralPath $runtimeRoot)) {
        $runtimeDirs = @(Get-ChildItem -LiteralPath $runtimeRoot -Directory -Recurse -Force -ErrorAction SilentlyContinue |
                         Sort-Object { $_.FullName.Length } -Descending | ForEach-Object { $_.FullName })
        foreach ($dir in $runtimeDirs) { Remove-EmptyDirectory $dir }
        Remove-EmptyDirectory $runtimeRoot
        Remove-EmptyDirectory (Split-Path -Parent $runtimeRoot)
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
        if (-not (Test-SafeDataRoot $countRoot (@($protectedRoots) + @([string]$countData.gameDir)))) { continue }
        $extraWorlds++
    }
    if ($RemoveWorldData -and $extraWorlds -gt 0) {
        Write-Host ""
        Write-Host "  -RemoveWorldData: EVERY world's journal goes, not only the first one's." -ForegroundColor Yellow
        Write-Host "  Each journal is this machine's record of the organisms that world took custody" -ForegroundColor Yellow
        Write-Host "  of and has not handed on. Nobody else holds a copy. Your worlds are NOT in it." -ForegroundColor Yellow
        Write-Host ("  Worlds beyond the first with a journal of their own: {0}." -f $extraWorlds) -ForegroundColor Yellow
        Write-Host ""
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
        if (-not (Test-SafeDataRoot $profileRoot (@($protectedRoots) + @([string]$profileData.gameDir)))) {
            [void]$kept.Add(("its data root is not a path this script will act on, so the whole " +
                             "profile stays : {0} (dataRoot '{1}')" -f $profilePath, $profileRoot))
            continue
        }
        # The first world's own files are removed with this install's own files
        # below, from the record rather than from the profile.
        if ($profileRoot -ne [string]$record.dataRoot) {
            $worldPending = New-Object System.Collections.ArrayList
            foreach ($leftover in @('peer-secret.txt', 'enrollment-pending.json',
                                    'sidecar.pid', 'game.pid', 'launcher.lock')) {
                Remove-Recorded -Path (Join-Path $profileRoot $leftover) -What "the world '$profileName'"
                [void]$worldPending.Add((Join-Path $profileRoot $leftover))
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
Remove-Recorded -Path $record.credential -What 'your map credential'
if ($record.certificate.storedCopy) {
    Remove-Recorded -Path $record.certificate.storedCopy -What "the copy of the map's certificate authority"
}
foreach ($leftover in @('sidecar.pid', 'game.pid')) {
    Remove-Recorded -Path (Join-Path $record.dataRoot $leftover)
}

if ($RemoveWorldData) {
    Write-Host ""
    Write-Host "  -RemoveWorldData: the journal goes too." -ForegroundColor Yellow
    Write-Host "  The journal is this machine's record of every organism it took custody of and" -ForegroundColor Yellow
    Write-Host "  has not handed on. Nobody else holds a copy, and no operator command can reach" -ForegroundColor Yellow
    Write-Host "  it. Deleting it drops whatever it still held. Your worlds are NOT in it and are" -ForegroundColor Yellow
    Write-Host "  not affected." -ForegroundColor Yellow
    Write-Host ""
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
    Say "the journal and the logs stay. Pass -RemoveWorldData to remove those as well."
} else {
    Remove-EmptyDirectory $record.dataRoot
}

# ---------------------------------------------------------------- the ledger

Write-Host ""
Write-Host "==== what was removed" -ForegroundColor Green
foreach ($item in $removed) { Say $item }
Write-Host ""
Write-Host "==== what was kept, and why"
foreach ($item in $kept) { Say $item }
Write-Host ""
Say "Untouched, in every run of this script: your worlds and their backups, in the"
Say "game's own folder under %USERPROFILE%\AppData\LocalLow\The Bibites. This package"
Say "never wrote outside its own directory there, and this script never reads them."
Write-Host ""
if ($DryRun) {
    Write-Host "Nothing was changed. Run it again without -DryRun to do it." -ForegroundColor Cyan
} else {
    if ($hasRuntime -and $record.runtime.managedByThisInstaller) {
        Write-Host "Done. Every unchanged managed game file was removed." -ForegroundColor Green
        Say "A changed or user-added runtime file remains in place, if the ledger above names one."
    } else {
        Write-Host "Done. The game is as the installer found it." -ForegroundColor Green
        Say "Steam can verify that for you: Properties -> Installed Files -> Verify integrity."
    }
}
Write-Host ""
Say "Leaving a map is a separate act from uninstalling, and it is one message to the"
Say "map's operator. Until they release your place, the map keeps it for you and every"
Say "organism addressed to it waits out its hold. See docs/participant/leave.md."
