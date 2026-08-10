<#
.SYNOPSIS
    Install the Bibites Multiverse far end - map slot 6, position (2,1) - on a
    Windows machine with no development tools.

.DESCRIPTION
    Runs on Windows PowerShell 5.1. It finds the Steam copy of The Bibites,
    checks that the game is the exact version this bundle was built against,
    installs BepInEx and the plugin, stores the shared LAN token in a file only
    you can read, and writes start-slot6.ps1 and stop-slot6.ps1 beside itself.

    It does not need administrator rights. It never starts the game.

    THIS MACHINE IS A FULL MEMBER OF THE MAP, not a spare. M4 runs a 3x2 grid of
    six worlds. The main machine runs five of them; BepInEx there hands out five
    log files and a sixth game on that machine never runs its plugin at all. The
    sixth world therefore lives here, where BepInEx is unused and the game gets
    its own log. It exports on ALL FOUR EDGES like every other slot: under D17
    two-way lanes every declared edge is both an export edge and an entry edge
    (contract-a.md §18 A38), and a 3x2 map is a torus, so all four have a
    neighbour.

.PARAMETER RelayHost
    The name or the IP address of the main machine, which runs the relay.

.PARAMETER TokenFile
    A file whose first line is the shared LAN token, copied from the main
    machine. The token is copied into this machine's own protected file.

.EXAMPLE
    .\setup-farend.ps1 -RelayHost 192.168.1.227 -TokenFile .\token.txt
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$RelayHost,
    [Parameter(Mandatory = $true)][string]$TokenFile,
    [string]$GameDir = '',
    [int]$RelayPort = 8795,
    [int]$SidecarPort = 8787,
    [int]$Slot = 6,
    [string]$Position = '2,1',
    [string]$ExportEdges = 'E,N,W,S',
    [string]$World = 'M4-Slot6',
    [string]$PeerId = 'slot-6',
    [int]$SaveMinutes = 10,
    [int]$SaveKeep = 6
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

# ---------------------------------------------------------------- 0. the map

# The position has to parse here, not on the wire: --position <col>,<row> is an
# advisory the relay arbitrates, and a malformed one is silently ignored, which
# reads as "the relay put me somewhere else for no reason".
if ($Position -notmatch '^\s*\d+\s*,\s*\d+\s*$') {
    Stop-Setup "-Position must be '<col>,<row>', for example '2,1'. It is '$Position'."
}
$Position = $Position -replace '\s', ''

# contract-a.md §15 A18: the edge list is comma-separated, at least one member,
# no duplicates. Catch a typo here rather than in the BepInEx log, where a bad
# set clears the whole list and disables the client with one error line.
#
# THE OPPOSITE-EDGE REFUSAL IS GONE (contract-a.md §18 A38, D17 two-way lanes).
# It rested on the one-way lane, where an edge was a capture band or a passive
# entry and never both, so 'E,W' was a contradiction. Every declared edge is now
# BOTH an export edge and an entry edge, and the conformant declaration is all
# four - which the old rule would have refused outright. Do not put it back.
$edgeList = @($ExportEdges -split '[,; \t]+' | Where-Object { $_ } | ForEach-Object { $_.ToUpperInvariant() })
if ($edgeList.Count -eq 0) {
    Stop-Setup "-ExportEdges names no edge. Use E, N, W or S, comma separated - normally 'E,N,W,S'."
}
foreach ($e in $edgeList) {
    if ($e -notin @('E', 'N', 'W', 'S')) { Stop-Setup "-ExportEdges holds '$e'. Use E, N, W or S." }
}
if (($edgeList | Select-Object -Unique).Count -ne $edgeList.Count) {
    Stop-Setup "-ExportEdges repeats an edge: '$ExportEdges'."
}
$ExportEdges = [string]::Join(',', $edgeList)

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
    Say "    (farend/make-farend-bundle.sh) and take the new bundle here."
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

# Only this account may read it. The token is the whole of M4's authentication:
# anybody on the LAN who has it can join the map. A new FileSecurity carries
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

$startName = "start-slot$Slot.ps1"
$stopName  = "stop-slot$Slot.ps1"
Step "6 of 6 - write $startName and $stopName"

$relayUrl   = "ws://${RelayHost}:$RelayPort/contract-b/v3"
$sidecarExe = Join-Path $Here 'multiverse-sidecar.exe'

$startBody = @'
# Generated by setup-farend.ps1. Start map slot @@SLOT@@ at position @@POSITION@@:
# the sidecar first, then the game.
#
#   .\@@STARTNAME@@             the sidecar, then the game
#   .\@@STARTNAME@@ -GameOnly   the game only, against a sidecar that is already
#                               running. This is the wake half of the main
#                               machine's arrival-pacing test: the sidecar keeps
#                               the journal and its custody while the world is
#                               away, so the backlog is there to drain.
#
# -Headless goes with either form. The world runs exactly as it would; only the
# picture is gone, and with it the need for this machine to have a screen.
[CmdletBinding()]
param([switch]$GameOnly, [switch]$Headless)
$ErrorActionPreference = 'Stop'

$GameDir     = '@@GAMEDIR@@'
$DataRoot    = '@@DATAROOT@@'
$RelayUrl    = '@@RELAYURL@@'
$SidecarExe  = '@@SIDECAREXE@@'
$PeerId      = '@@PEERID@@'
$Slot        = '@@SLOT@@'
$Position    = '@@POSITION@@'
$ExportEdges = '@@EXPORTEDGES@@'
$SidecarPort = '@@SIDECARPORT@@'
$World       = '@@WORLD@@'
$SaveMinutes = '@@SAVEMINUTES@@'
$SaveKeep    = '@@SAVEKEEP@@'

$dataDir   = Join-Path $DataRoot 'data-slot-@@SLOT@@'
$logDir    = Join-Path $DataRoot 'logs'
$tokenFile = Join-Path $DataRoot 'token.txt'
$log       = Join-Path $logDir 'sidecar-slot@@SLOT@@.log'
$cmdDir    = Join-Path $env:TEMP 'bibites-m4'
New-Item -ItemType Directory -Force -Path $dataDir, $logDir, $cmdDir | Out-Null

# The mod reads its whole configuration from the environment of the game
# process. A Windows process inherits it from this script, so there is nothing
# like WSLENV to declare here.
#
# MULTIVERSE_EXPORT_EDGES is the M4 plural name. All four lanes of this slot
# wrap: a 3x2 map is a torus, so every slot declares all four edges and the
# sidecar decides from the relay's map which of them EDGE_STATUS actually opens.
# Under D17 each declared edge is both an export edge and an entry edge
# (contract-a.md §18 A38). The mod's own default is all four when this is unset;
# it is set anyway, so the value this setup was run with is what the world uses.
$env:MULTIVERSE_EXPORT_EDGES = $ExportEdges
$env:MULTIVERSE_RING_SLOT    = $Slot
$env:MULTIVERSE_SIDECAR_PORT = $SidecarPort
$env:MULTIVERSE_WORLD        = $World
$env:MULTIVERSE_CMD_FILE     = Join-Path $cmdDir 'cmd-@@SLOT@@.txt'
# The periodic save is mod configuration, so it is set HERE and nowhere else.
# The main machine cannot save this world and never tries: it only reads the
# receipt, which rides every HEARTBEAT and reaches the status page.
$env:MULTIVERSE_SAVE_MINUTES = $SaveMinutes
$env:MULTIVERSE_SAVE_KEEP    = $SaveKeep
$env:MULTIVERSE_SAVE_ON_QUIT = 'true'
$env:MULTIVERSE_PORTAL       = 'true'
$env:MULTIVERSE_PORTAL_FLOURISHES = 'true'

$sidecarPidFile = Join-Path $DataRoot 'sidecar.pid'
$gamePidFile    = Join-Path $DataRoot 'game.pid'

# Both starts below launch the game from this one description, so the -GameOnly
# path and the full path cannot drift apart. -batchmode and -nographics are
# Unity's own switches, handled before the game sees them: its parser reads only
# -steam and ignores everything else.
$gameLaunch = @{
    FilePath         = (Join-Path $GameDir 'The Bibites.exe')
    WorkingDirectory = $GameDir
    PassThru         = $true
}
if ($Headless) { $gameLaunch['ArgumentList'] = @('-batchmode', '-nographics') }

if ($GameOnly) {
    if (-not (Get-Process -Name 'multiverse-sidecar' -ErrorAction SilentlyContinue)) {
        Write-Host "-GameOnly needs the sidecar already running. It is not." -ForegroundColor Red
        Write-Host "Run .\@@STARTNAME@@ with no switch."
        exit 1
    }
    if (Get-Process -Name 'The Bibites' -ErrorAction SilentlyContinue) {
        Write-Host "The game is already running."
        exit 1
    }
    $game = Start-Process @gameLaunch
    Set-Content -Path $gamePidFile -Value $game.Id -Encoding ASCII
    Write-Host "game started (pid $($game.Id)) against the running sidecar; it loads '$World' by itself."
    Write-Host "The sidecar replays every organism it took custody of while the world was away."
    exit 0
}

if (Get-Process -Name 'multiverse-sidecar' -ErrorAction SilentlyContinue) {
    Write-Host "A sidecar is already running. Run .\@@STOPNAME@@ first,"
    Write-Host "or .\@@STARTNAME@@ -GameOnly to start only the game."
    exit 1
}

Remove-Item -Path $log, "$log.out" -ErrorAction SilentlyContinue
# --position is ADVISORY (contract-b-m4.md §7.2). The main machine also reserves
# this peerId at the same coordinate before the relay serves, so the map forms in
# any start order and this flag only matters on a relay that has no reservation.
$sidecarArgs = @(
    '--listen',     "127.0.0.1:$SidecarPort",
    '--relay',      $RelayUrl,
    '--peer-id',    $PeerId,
    '--position',   $Position,
    '--data-dir',   $dataDir,
    '--token-file', $tokenFile
)
$sidecar = Start-Process -FilePath $SidecarExe -PassThru -WindowStyle Hidden -WorkingDirectory $DataRoot -ArgumentList $sidecarArgs -RedirectStandardError $log -RedirectStandardOutput "$log.out"
Set-Content -Path $sidecarPidFile -Value $sidecar.Id -Encoding ASCII
Write-Host "sidecar started (pid $($sidecar.Id)) -> $RelayUrl"
Write-Host "waiting for the relay to grant slot $Slot at position $Position ..."

$deadline = (Get-Date).AddSeconds(60)
$granted  = $null
$refused  = $null
while ((Get-Date) -lt $deadline) {
    if (Test-Path $log) {
        $granted = Select-String -Path $log -Pattern 'contract B: slot granted' -SimpleMatch | Select-Object -Last 1
        if ($granted) { break }
        $refused = Select-String -Path $log -Pattern 'placement claim refused', 'HTTP 401' | Select-Object -Last 1
        if ($refused) { break }
    }
    if ($sidecar.HasExited) { break }
    Start-Sleep -Milliseconds 500
}

if ($granted) {
    Write-Host ""
    Write-Host "SLOT GRANTED:" -ForegroundColor Green
    Write-Host "  $($granted.Line)"
} else {
    Write-Host ""
    Write-Host "The relay did not grant a slot." -ForegroundColor Red
    if ($refused) { Write-Host "  $($refused.Line)" }
    if (Test-Path $log) { Get-Content -Path $log -Tail 20 | ForEach-Object { Write-Host "  $_" } }
    Write-Host ""
    Write-Host "  The four usual causes, in order:"
    Write-Host "   1. The firewall or the port forward on the main machine does not carry TCP @@RELAYPORT@@."
    Write-Host "      M4 moved the relay from 8790 to @@RELAYPORT@@, and both rules had to move with it."
    Write-Host "   2. $RelayUrl names the wrong machine, or the relay is not running."
    Write-Host "   3. The token here does not match the token on the main machine (HTTP 401)."
    Write-Host "   4. The game version here does not match the map (version_incompatible)."
    Write-Host ""
    Write-Host "  The game was NOT started. Run .\@@STOPNAME@@, then try again."
    exit 1
}

$game = Start-Process @gameLaunch
Set-Content -Path $gamePidFile -Value $game.Id -Encoding ASCII
Write-Host ""
Write-Host "game started (pid $($game.Id)); it loads the world '$World' by itself,"
Write-Host "and seeds it on the first start. It saves itself every $SaveMinutes minutes."
Write-Host "logs: $log  and  $GameDir\BepInEx\LogOutput.log"
Write-Host "Leave both running. Run .\@@STOPNAME@@ when the test is over."
'@

$stopBody = @'
# Generated by setup-farend.ps1. Stop map slot @@SLOT@@: the game first, then the
# sidecar.
#
#   .\@@STOPNAME@@             the game and the sidecar
#   .\@@STOPNAME@@ -GameOnly   the game only. The sidecar stays up, keeps its
#                              relay session and keeps taking custody of arrivals
#                              into its journal. This is the dark half of the main
#                              machine's arrival-pacing test; wake it with
#                              .\@@STARTNAME@@ -GameOnly.
[CmdletBinding()]
param([switch]$GameOnly)
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

Stop-Recorded (Join-Path $DataRoot 'game.pid') 'the game'
Stop-Process -Name 'The Bibites' -Force
Start-Sleep -Seconds 1

if ($GameOnly) {
    $sc = @(Get-Process -Name 'multiverse-sidecar' -ErrorAction SilentlyContinue)
    Write-Host ("the world is down; sidecar processes still running: {0} (want 1)" -f $sc.Count)
    Write-Host "The sidecar keeps its slot and its journal. Arrivals accumulate there and"
    Write-Host "are delivered, paced, when the world comes back."
    exit 0
}

Stop-Recorded (Join-Path $DataRoot 'sidecar.pid') 'the sidecar'
Stop-Process -Name 'multiverse-sidecar' -Force
Start-Sleep -Seconds 1

$left = @(Get-Process -Name 'The Bibites', 'multiverse-sidecar' -ErrorAction SilentlyContinue)
Write-Host ("slot @@SLOT@@ processes still running: {0} (want 0)" -f $left.Count)
Write-Host "The journal in $DataRoot\data-slot-@@SLOT@@ is kept. Do not delete it: it is this"
Write-Host "machine's record of every organism it is holding."
'@

$startPath = Join-Path $Here $startName
$stopPath  = Join-Path $Here $stopName

function Expand-Template {
    param([string]$Body)
    return $Body.Replace('@@GAMEDIR@@',     $GameDir).
                 Replace('@@DATAROOT@@',    $DataRoot).
                 Replace('@@RELAYURL@@',    $relayUrl).
                 Replace('@@RELAYPORT@@',   [string]$RelayPort).
                 Replace('@@SIDECAREXE@@',  $sidecarExe).
                 Replace('@@PEERID@@',      $PeerId).
                 Replace('@@SLOT@@',        [string]$Slot).
                 Replace('@@POSITION@@',    $Position).
                 Replace('@@EXPORTEDGES@@', $ExportEdges).
                 Replace('@@SIDECARPORT@@', [string]$SidecarPort).
                 Replace('@@WORLD@@',       $World).
                 Replace('@@SAVEMINUTES@@', [string]$SaveMinutes).
                 Replace('@@SAVEKEEP@@',    [string]$SaveKeep).
                 Replace('@@STARTNAME@@',   $startName).
                 Replace('@@STOPNAME@@',    $stopName)
}

Set-Content -Path $startPath -Value (Expand-Template $startBody) -Encoding ASCII
Set-Content -Path $stopPath  -Value (Expand-Template $stopBody)  -Encoding ASCII
Say "wrote $startPath"
Say "wrote $stopPath"

Write-Host ""
Write-Host "Setup is complete." -ForegroundColor Green
Say "relay        : $relayUrl"
Say "slot         : $Slot   position: $Position   peer id: $PeerId"
Say "world        : $World   export edges: $ExportEdges"
Say "sidecar port : $SidecarPort (loopback on this machine only)"
Say "saves        : every $SaveMinutes minutes, keeping $SaveKeep backups"
Say "data         : $DataRoot"
Write-Host ""
Say "Next: run  .\$startName"
