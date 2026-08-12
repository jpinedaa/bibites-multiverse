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
      * Start-Multiverse.ps1 and Stop-Multiverse.ps1
      * your map credential, and the copy of a private map's certificate
        authority
      * that certificate authority from your own user trust store, if the
        installer imported it and only then

    WHAT IT KEEPS, DELIBERATELY

      * YOUR WORLDS AND THEIR BACKUPS. They are the game's own files, in the
        game's own folder under %USERPROFILE%\AppData\LocalLow\The Bibites, and
        nothing in this package has ever written outside its own directory
        there. This script does not go near them
      * YOUR JOURNAL - the record of every organism this machine took custody of
        and has not handed on. Pass -RemoveWorldData to delete it too, and read
        the warning that prints before it happens
      * any file the installer did not create, including another mod's plugin

.PARAMETER DataRoot
    Where this install kept its files. Defaults to
    %LOCALAPPDATA%\BibitesMultiverse.

.PARAMETER RecordFile
    The install record, if it is not in the usual place.

.PARAMETER RemoveWorldData
    Also delete the journal, the logs and the data directory. The journal may
    still hold organisms other worlds handed to this one.

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

function Remove-EmptyDirectory {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { return }
    $left = @(Get-ChildItem -LiteralPath $Path -Force -ErrorAction SilentlyContinue)
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

# Both checks are on THIS install's own paths rather than on a process name: a
# machine may hold a second copy of the game or a second world, and only the one
# this record describes has to be stopped. A process whose path cannot be read
# counts against the check, because the safe answer to "I cannot tell" is to stop.
function Test-ProcessUnder {
    param([string]$Name, [string]$Path)
    foreach ($process in @(Get-Process -Name $Name -ErrorAction SilentlyContinue)) {
        $exe = ''
        try { $exe = $process.Path } catch { $exe = '' }
        if (-not $exe) { return $true }
        if ($exe.StartsWith($Path, [System.StringComparison]::OrdinalIgnoreCase)) { return $true }
    }
    return $false
}

if (Test-ProcessUnder 'The Bibites' $record.gameDir) {
    Stop-Uninstall ("The Bibites is running from $($record.gameDir). Close it first; Windows holds " +
                    "the plugin open while the game runs. Nothing was removed.")
}
if (Test-ProcessUnder 'multiverse-sidecar' $record.kitDir) {
    Stop-Uninstall "This install's sidecar is running. Run .\Stop-Multiverse.ps1 first. Nothing was removed."
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

# ---------------------------------------------------------------- this install's own files

Step "this install's own files"

foreach ($script in @($record.generated)) {
    Remove-Recorded -Path $script -What 'written by the installer'
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
    Write-Host "Done. The game is as the installer found it." -ForegroundColor Green
    Say "Steam can verify that for you: Properties -> Installed Files -> Verify integrity."
}
Write-Host ""
Say "Leaving a map is a separate act from uninstalling, and it is one message to the"
Say "map's operator. Until they release your place, the map keeps it for you and every"
Say "organism addressed to it waits out its hold. See docs/participant/leave.md."
