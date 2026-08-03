<#
.SYNOPSIS
    Install the Bibites Multiverse far end - ring slot 2 - on a Windows machine
    with no development tools.

.DESCRIPTION
    Runs on Windows PowerShell 5.1. It finds the Steam copy of The Bibites,
    checks that the game is the exact version this bundle was built against,
    installs BepInEx and the plugin, stores the shared LAN token in a file only
    you can read, and writes start-slot2.ps1 and stop-slot2.ps1 beside itself.

    It does not need administrator rights. It never starts the game.

.PARAMETER RelayHost
    The name or the IP address of the main machine, which runs the relay.

.PARAMETER TokenFile
    A file whose first line is the shared LAN token, copied from the main
    machine. The token is copied into this machine's own protected file.

.EXAMPLE
    .\setup-farend.ps1 -RelayHost 192.168.1.20 -TokenFile .\token.txt
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$RelayHost,
    [Parameter(Mandatory = $true)][string]$TokenFile,
    [string]$GameDir = '',
    [int]$RelayPort = 8790,
    [int]$SidecarPort = 8787,
    [int]$Slot = 2,
    [string]$World = 'M3-Slot2',
    [string]$PeerId = 'slot-2'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

# ---------------------------------------------------------------- the pin
#
# This bundle is compiled against one game build. The plugin patches game
# methods by name and signature, so a different build can fail at load, or -
# worse - load and behave differently. The hash below is the SHA-256 of
# BibitesAssembly.dll on the main machine, which is the reference the mod was
# built against. It is the version gate, and it is deliberately exact.
$GameVersion       = '0.6.3.1'
$SteamAppId        = '2736860'
$SteamBuildId      = '22383127'
$AssemblySha256    = '12455E485199CDBCAEA5978B8B0095EEDCBDD09D1FB87EFD65CCACB15D96E7EE'
$BepInExVersion    = '5.4.23.3'
$BepInExZipName    = 'BepInEx_win_x64_5.4.23.3.zip'
$BepInExZipSha256  = '41A089E5B1B1F0713B331346BAF6677B1184C69EABEBF51101097954E854C749'

$Here     = Split-Path -Parent $MyInvocation.MyCommand.Path
$DataRoot = Join-Path $env:LOCALAPPDATA 'BibitesMultiverse'

function Say  { param([string]$m) Write-Host "     $m" }
function Step { param([string]$m) Write-Host ""; Write-Host "==== $m" }
function Stop-Setup {
    param([string]$m)
    Write-Host ""
    Write-Host "STOP: $m" -ForegroundColor Red
    exit 1
}

function Get-Sha256 {
    param([string]$Path)
    return (Get-FileHash -Path $Path -Algorithm SHA256).Hash.ToUpperInvariant()
}

# ---------------------------------------------------------------- find the game

function Get-SteamRoots {
    $roots = New-Object System.Collections.ArrayList

    foreach ($key in @('HKCU:\Software\Valve\Steam', 'HKLM:\SOFTWARE\WOW6432Node\Valve\Steam',
                       'HKLM:\SOFTWARE\Valve\Steam')) {
        try {
            $item = Get-ItemProperty -Path $key -ErrorAction Stop
        } catch {
            continue
        }
        foreach ($name in @('InstallPath', 'SteamPath')) {
            $value = $null
            if ($item.PSObject.Properties.Match($name).Count -gt 0) { $value = $item.$name }
            if ($value) { [void]$roots.Add(([string]$value).Replace('/', '\')) }
        }
    }

    foreach ($guess in @("${env:ProgramFiles(x86)}\Steam", "$env:ProgramFiles\Steam",
                         "$env:SystemDrive\Steam")) {
        if ($guess) { [void]$roots.Add($guess) }
    }

    # A second drive is normal. Every extra Steam library is listed in the
    # library index of the first one.
    $extra = New-Object System.Collections.ArrayList
    foreach ($root in $roots) {
        $vdf = Join-Path $root 'steamapps\libraryfolders.vdf'
        if (-not (Test-Path $vdf)) { continue }
        foreach ($line in (Get-Content -Path $vdf)) {
            $m = [regex]::Match($line, '"path"\s+"(.+?)"')
            if ($m.Success) { [void]$extra.Add($m.Groups[1].Value.Replace('\\', '\')) }
        }
    }
    foreach ($e in $extra) { [void]$roots.Add($e) }

    return ($roots | Where-Object { $_ } | Select-Object -Unique)
}

function Find-GameDir {
    foreach ($root in (Get-SteamRoots)) {
        $candidate = Join-Path $root 'steamapps\common\The Bibites'
        if (Test-Path (Join-Path $candidate 'The Bibites.exe')) { return $candidate }
    }
    return ''
}

# ---------------------------------------------------------------- 1. the game

Step "1 of 6 - find The Bibites"
# Windows keeps the plugin file open while the game runs, and the copy in step 4
# then fails with an unreadable IOException. Stop before anything is half-done.
if (Get-Process -Name 'The Bibites' -ErrorAction SilentlyContinue) {
    Stop-Setup "The Bibites is running. Close the game, then run this script again."
}
if (-not $GameDir) { $GameDir = Find-GameDir }
if (-not $GameDir) {
    Stop-Setup ("Steam has no copy of The Bibites on this machine, or it is in a place this " +
                "script does not know. Install the game, or run this script again with " +
                "-GameDir 'D:\path\to\The Bibites'.")
}
if (-not (Test-Path (Join-Path $GameDir 'The Bibites.exe'))) {
    Stop-Setup "There is no 'The Bibites.exe' in $GameDir."
}
Say "game directory: $GameDir"

Step "2 of 6 - check the game version"
$assembly = Join-Path $GameDir 'The Bibites_Data\Managed\BibitesAssembly.dll'
if (-not (Test-Path $assembly)) { Stop-Setup "The game assembly is missing: $assembly" }
$got = Get-Sha256 $assembly
if ($got -ne $AssemblySha256) {
    Write-Host ""
    Write-Host "The game on this machine is NOT the build this bundle was made for." -ForegroundColor Red
    Write-Host ""
    Say "expected game version : $GameVersion (Steam app $SteamAppId, buildid $SteamBuildId)"
    Say "expected DLL SHA-256  : $AssemblySha256"
    Say "this machine's SHA-256: $got"
    Write-Host ""
    Say "The plugin patches game methods by name and signature. A different build can"
    Say "fail to load, or load and behave differently, and the relay then refuses this"
    Say "peer at claim time - which looks exactly like a dead computer."
    Write-Host ""
    Say "Two ways forward:"
    Say " 1. Let Steam update the game on the MAIN machine as well. Re-sync the"
    Say "    references there (bibites-mod/sync-game-refs.sh), rebuild the bundle"
    Say "    (farend/make-farend-bundle.sh) and copy the new bundle here."
    Say " 2. Or put this machine back on game version $GameVersion."
    Stop-Setup "the game version does not match; nothing was installed."
}
Say "BibitesAssembly.dll matches the pin: game version $GameVersion"

# ---------------------------------------------------------------- 3. BepInEx

Step "3 of 6 - BepInEx $BepInExVersion"
$bepinexCore = Join-Path $GameDir 'BepInEx\core\BepInEx.dll'
if (Test-Path $bepinexCore) {
    Say "BepInEx is already installed; left alone."
} else {
    $zip = Join-Path $Here $BepInExZipName
    if (-not (Test-Path $zip)) { Stop-Setup "The bundle is incomplete: $BepInExZipName is missing." }
    $zipHash = Get-Sha256 $zip
    if ($zipHash -ne $BepInExZipSha256) {
        Stop-Setup "$BepInExZipName is not the pinned file (SHA-256 $zipHash). Copy the bundle again."
    }
    Expand-Archive -Path $zip -DestinationPath $GameDir -Force
    if (-not (Test-Path $bepinexCore)) { Stop-Setup "BepInEx did not unpack into $GameDir." }
    Say "BepInEx $BepInExVersion installed into the game directory."
    Say "The first game start writes the BepInEx configuration. That start is slower."
}

# ---------------------------------------------------------------- 4. the plugin

Step "4 of 6 - the multiverse plugin"
$pluginSrc = Join-Path $Here 'BibitesMultiverse.dll'
if (-not (Test-Path $pluginSrc)) { Stop-Setup "The bundle is incomplete: BibitesMultiverse.dll is missing." }
$plugins = Join-Path $GameDir 'BepInEx\plugins'
New-Item -ItemType Directory -Force -Path $plugins | Out-Null
Copy-Item -Path $pluginSrc -Destination $plugins -Force
Say "BibitesMultiverse.dll -> $plugins"

$sidecarSrc = Join-Path $Here 'multiverse-sidecar.exe'
if (-not (Test-Path $sidecarSrc)) { Stop-Setup "The bundle is incomplete: multiverse-sidecar.exe is missing." }

# ---------------------------------------------------------------- 5. the token

Step "5 of 6 - the shared LAN token"
if (-not (Test-Path $TokenFile)) { Stop-Setup "No token file at $TokenFile." }
$token = ''
foreach ($line in (Get-Content -Path $TokenFile)) {
    $candidate = $line.Trim()
    if ($candidate) { $token = $candidate; break }
}
if ($token.Length -lt 16 -or $token.Length -gt 256) {
    Stop-Setup "The token is $($token.Length) characters. It must be 16 to 256."
}
if ($token -notmatch '^[\x21-\x7e]+$') {
    Stop-Setup "The token must be printable ASCII with no spaces."
}

New-Item -ItemType Directory -Force -Path $DataRoot | Out-Null
$tokenPath = Join-Path $DataRoot 'token.txt'
# Written fresh every time. Re-writing over an already protected file makes the
# permission change below need a privilege an ordinary account does not have.
Remove-Item -Path $tokenPath -Force -ErrorAction SilentlyContinue
Set-Content -Path $tokenPath -Value $token -Encoding ASCII

# Only this account may read it. The token is the whole of M3's authentication:
# anybody on the LAN who has it can join the ring. A new FileSecurity carries
# only the permission list, so this touches no other part of the file's
# security and needs no administrator rights.
$me = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
try {
    $sec = New-Object System.Security.AccessControl.FileSecurity
    $sec.SetAccessRuleProtection($true, $false)
    $sec.AddAccessRule((New-Object System.Security.AccessControl.FileSystemAccessRule($me, 'FullControl', 'Allow')))
    (Get-Item -Path $tokenPath).SetAccessControl($sec)
    Say "token stored in $tokenPath, readable by $me only"
} catch {
    Say "token stored in $tokenPath"
    Say "WARNING: the permissions could not be tightened: $($_.Exception.Message)"
    Say "The file is inside your own profile, which other accounts cannot read by default."
}

# ---------------------------------------------------------------- 6. the scripts

Step "6 of 6 - write start-slot2.ps1 and stop-slot2.ps1"

$relayUrl   = "ws://${RelayHost}:$RelayPort/contract-b/v2"
$sidecarExe = Join-Path $Here 'multiverse-sidecar.exe'

$startBody = @'
# Generated by setup-farend.ps1. Start ring slot 2: the sidecar first, then the game.
$ErrorActionPreference = 'Stop'

$GameDir     = '@@GAMEDIR@@'
$DataRoot    = '@@DATAROOT@@'
$RelayUrl    = '@@RELAYURL@@'
$SidecarExe  = '@@SIDECAREXE@@'
$PeerId      = '@@PEERID@@'
$Slot        = '@@SLOT@@'
$SidecarPort = '@@SIDECARPORT@@'
$World       = '@@WORLD@@'

$dataDir   = Join-Path $DataRoot 'data-slot-2'
$logDir    = Join-Path $DataRoot 'logs'
$tokenFile = Join-Path $DataRoot 'token.txt'
$log       = Join-Path $logDir 'sidecar-slot2.log'
$cmdDir    = Join-Path $env:TEMP 'bibites-m3'
New-Item -ItemType Directory -Force -Path $dataDir, $logDir, $cmdDir | Out-Null

# The mod reads its whole configuration from the environment of the game
# process. A Windows process inherits it from this script, so there is nothing
# like WSLENV to declare here.
$env:MULTIVERSE_EXPORT_EDGE  = 'E'
$env:MULTIVERSE_RING_SLOT    = $Slot
$env:MULTIVERSE_SIDECAR_PORT = $SidecarPort
$env:MULTIVERSE_WORLD        = $World
$env:MULTIVERSE_CMD_FILE     = Join-Path $cmdDir 'cmd-2.txt'

$sidecarPidFile = Join-Path $DataRoot 'sidecar.pid'
$gamePidFile    = Join-Path $DataRoot 'game.pid'
if (Get-Process -Name 'multiverse-sidecar' -ErrorAction SilentlyContinue) {
    Write-Host "A sidecar is already running. Run .\stop-slot2.ps1 first."
    exit 1
}

Remove-Item -Path $log, "$log.out" -ErrorAction SilentlyContinue
$sidecarArgs = @(
    '--listen',     "127.0.0.1:$SidecarPort",
    '--relay',      $RelayUrl,
    '--peer-id',    $PeerId,
    '--data-dir',   $dataDir,
    '--token-file', $tokenFile
)
$sidecar = Start-Process -FilePath $SidecarExe -PassThru -WindowStyle Hidden -WorkingDirectory $DataRoot -ArgumentList $sidecarArgs -RedirectStandardError $log -RedirectStandardOutput "$log.out"
Set-Content -Path $sidecarPidFile -Value $sidecar.Id -Encoding ASCII
Write-Host "sidecar started (pid $($sidecar.Id)) -> $RelayUrl"
Write-Host "waiting for the relay to grant ring slot $Slot ..."

$deadline = (Get-Date).AddSeconds(60)
$granted  = $null
$refused  = $null
while ((Get-Date) -lt $deadline) {
    if (Test-Path $log) {
        $granted = Select-String -Path $log -Pattern 'ring slot granted' -SimpleMatch | Select-Object -Last 1
        if ($granted) { break }
        $refused = Select-String -Path $log -Pattern 'ring claim refused' -SimpleMatch | Select-Object -Last 1
        if ($refused) { break }
    }
    if ($sidecar.HasExited) { break }
    Start-Sleep -Milliseconds 500
}

if ($granted) {
    Write-Host ""
    Write-Host "RING SLOT GRANTED:" -ForegroundColor Green
    Write-Host "  $($granted.Line)"
} else {
    Write-Host ""
    Write-Host "The relay did not grant a ring slot." -ForegroundColor Red
    if ($refused) { Write-Host "  $($refused.Line)" }
    if (Test-Path $log) { Get-Content -Path $log -Tail 20 | ForEach-Object { Write-Host "  $_" } }
    Write-Host ""
    Write-Host "  The four usual causes, in order:"
    Write-Host "   1. The firewall on the main machine blocks TCP 8790."
    Write-Host "   2. $RelayUrl names the wrong machine, or the relay is not running."
    Write-Host "   3. The token here does not match the token on the main machine (401)."
    Write-Host "   4. The game version here does not match the ring (version_incompatible)."
    Write-Host ""
    Write-Host "  The game was NOT started. Run .\stop-slot2.ps1, then try again."
    exit 1
}

$game = Start-Process -FilePath (Join-Path $GameDir 'The Bibites.exe') -PassThru -WorkingDirectory $GameDir
Set-Content -Path $gamePidFile -Value $game.Id -Encoding ASCII
Write-Host ""
Write-Host "game started (pid $($game.Id)); it loads the world '$World' by itself."
Write-Host "logs: $log  and  $GameDir\BepInEx\LogOutput.log"
Write-Host "Leave both running. Run .\stop-slot2.ps1 when the test is over."
'@

$stopBody = @'
# Generated by setup-farend.ps1. Stop ring slot 2: the game first, then the sidecar.
$ErrorActionPreference = 'SilentlyContinue'

$DataRoot = '@@DATAROOT@@'

function Stop-Recorded {
    param([string]$File, [string]$Name)
    if (Test-Path $File) {
        $id = (Get-Content -Path $File | Select-Object -First 1)
        if ($id) {
            Stop-Process -Id ([int]$id) -Force
            Write-Host "stopped $Name (pid $id)"
        }
        Remove-Item -Path $File -Force
    }
}

Stop-Recorded (Join-Path $DataRoot 'game.pid')    'the game'
Stop-Recorded (Join-Path $DataRoot 'sidecar.pid') 'the sidecar'

# Anything an earlier run left behind.
Stop-Process -Name 'The Bibites' -Force
Stop-Process -Name 'multiverse-sidecar' -Force
Start-Sleep -Seconds 1

$left = @(Get-Process -Name 'The Bibites', 'multiverse-sidecar' -ErrorAction SilentlyContinue)
Write-Host ("slot 2 processes still running: {0} (want 0)" -f $left.Count)
Write-Host "The journal in $DataRoot\data-slot-2 is kept. Do not delete it: it is this"
Write-Host "machine's record of every organism it is holding."
'@

$startPath = Join-Path $Here 'start-slot2.ps1'
$stopPath  = Join-Path $Here 'stop-slot2.ps1'

$start = $startBody.Replace('@@GAMEDIR@@',     $GameDir).
                    Replace('@@DATAROOT@@',    $DataRoot).
                    Replace('@@RELAYURL@@',    $relayUrl).
                    Replace('@@SIDECAREXE@@',  $sidecarExe).
                    Replace('@@PEERID@@',      $PeerId).
                    Replace('@@SLOT@@',        [string]$Slot).
                    Replace('@@SIDECARPORT@@', [string]$SidecarPort).
                    Replace('@@WORLD@@',       $World)
Set-Content -Path $startPath -Value $start -Encoding ASCII
Set-Content -Path $stopPath  -Value $stopBody.Replace('@@DATAROOT@@', $DataRoot) -Encoding ASCII
Say "wrote $startPath"
Say "wrote $stopPath"

Write-Host ""
Write-Host "Setup is complete." -ForegroundColor Green
Say "relay      : $relayUrl"
Say "ring slot  : $Slot   world: $World   sidecar port: $SidecarPort"
Say "data       : $DataRoot"
Write-Host ""
Say "Next: run  .\start-slot2.ps1"
