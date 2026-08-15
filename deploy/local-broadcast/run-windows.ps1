param([switch]$ArgumentQuotingSelfTest)

$ErrorActionPreference = 'Stop'

function ConvertTo-WindowsArgument([string]$Value) {
    if ($Value.Length -gt 0 -and $Value -notmatch '[\s"]') { return $Value }

    $result = '"'
    $slashes = 0
    foreach ($character in $Value.ToCharArray()) {
        if ($character -eq '\') {
            $slashes++
            continue
        }
        if ($character -eq '"') {
            $result += ('\' * (($slashes * 2) + 1)) + '"'
            $slashes = 0
            continue
        }
        if ($slashes -gt 0) {
            $result += '\' * $slashes
            $slashes = 0
        }
        $result += $character
    }
    if ($slashes -gt 0) { $result += '\' * ($slashes * 2) }
    return $result + '"'
}

function Start-NativeProcess {
    param(
        [string]$FilePath,
        [string]$WorkingDirectory,
        [string[]]$ArgumentList = @(),
        [switch]$Hidden
    )
    $startInfo = New-Object System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = $FilePath
    $startInfo.WorkingDirectory = $WorkingDirectory
    $startInfo.UseShellExecute = $false
    $startInfo.CreateNoWindow = [bool]$Hidden
    if ($Hidden) { $startInfo.WindowStyle = [System.Diagnostics.ProcessWindowStyle]::Hidden }
    $quoted = @($ArgumentList | ForEach-Object { ConvertTo-WindowsArgument ([string]$_) })
    $startInfo.Arguments = [string]::Join(' ', $quoted)
    $process = New-Object System.Diagnostics.Process
    $process.StartInfo = $startInfo
    if (-not $process.Start()) { throw "Windows did not start $FilePath" }
    return $process
}

if ($ArgumentQuotingSelfTest) {
    $fixtureRoot = Join-Path ([IO.Path]::GetTempPath()) ('bibites-argv-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $fixtureRoot | Out-Null
    try {
        $childPath = Join-Path $fixtureRoot 'argument child.ps1'
        $outputPath = Join-Path $fixtureRoot 'observed arguments.json'
        @'
param([string]$OutputPath, [string]$First, [string]$Second, [string]$Third, [string]$Fourth)
ConvertTo-Json -InputObject @($First, $Second, $Third, $Fourth) -Compress |
    Set-Content -LiteralPath $OutputPath -Encoding UTF8
'@ | Set-Content -LiteralPath $childPath -Encoding UTF8
        $expected = @(
            'value with spaces',
            'C:\Program Files\Bibites Multiverse\trail\',
            'quote"inside',
            'simple'
        )
        $hostExe = (Get-Process -Id $PID).Path
        $childArguments = @('-NoProfile', '-File', $childPath, $outputPath) + $expected
        $child = Start-NativeProcess -FilePath $hostExe -WorkingDirectory $fixtureRoot `
            -ArgumentList $childArguments -Hidden
        if (-not $child.WaitForExit(30000)) {
            $child.Kill()
            throw 'The spaced-argument child process did not finish'
        }
        if ($child.ExitCode -ne 0) { throw "The spaced-argument child process exited with code $($child.ExitCode)" }
        $observedJson = (Get-Content -LiteralPath $outputPath -Raw).Trim()
        $expectedJson = ConvertTo-Json -InputObject $expected -Compress
        if ($observedJson -ne $expectedJson) {
            throw 'The child process received changed arguments'
        }
    } finally {
        Remove-Item -LiteralPath $fixtureRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
    Write-Host 'Windows spaced-argument fixture: PASS'
    exit 0
}

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$configPath = Join-Path $root 'config.env'
if (-not (Test-Path -LiteralPath $configPath)) { throw "Missing $configPath" }

$config = @{}
foreach ($line in Get-Content -LiteralPath $configPath) {
    if (-not $line -or $line.StartsWith('#')) { continue }
    $pair = $line.Split('=', 2)
    if ($pair.Count -ne 2) { throw "Invalid configuration line in $configPath" }
    $config[$pair[0]] = $pair[1]
}
foreach ($name in @('GameDir', 'Obs', 'WorldName', 'PublishPort', 'SidecarExe', 'DataRoot',
                    'RelayUrl', 'PeerId', 'SidecarPort', 'ExportEdges', 'ExcludeSpecies')) {
    if (-not $config.ContainsKey($name) -or -not $config[$name]) { throw "Missing $name in $configPath" }
}
if ($config.ExportEdges -eq 'none') {
    throw 'ExportEdges is none, which would take the broadcast world off the map'
}

$logs = Join-Path $root 'logs'
$state = Join-Path $root 'state'
New-Item -ItemType Directory -Force -Path $logs, $state | Out-Null

$connected = $false
for ($attempt = 0; $attempt -lt 120; $attempt++) {
    try {
        $client = [Net.Sockets.TcpClient]::new()
        $client.Connect('127.0.0.1', [int]$config.PublishPort)
        $client.Dispose()
        $connected = $true
        break
    } catch {
        if ($client) { $client.Dispose() }
        Start-Sleep -Seconds 1
    }
}
if (-not $connected) { throw 'The private RTMP tunnel did not open' }

# The broadcast world is an ordinary participant of the public map. The camera
# is the only thing that makes it special, so every migration setting here is
# the one a participant install writes.
$dataDir = Join-Path $config.DataRoot 'data'
$credentialFile = Join-Path $config.DataRoot 'peer-secret.txt'
$sidecarLog = Join-Path $logs 'sidecar.log'
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
if (-not (Test-Path -LiteralPath $credentialFile)) {
    throw "There is no map credential at $credentialFile. Run install.sh again."
}
$durablePeerFile = Join-Path $dataDir 'peer-id'
if (-not (Test-Path -LiteralPath $durablePeerFile -PathType Leaf)) {
    throw "There is no durable peer identity at $durablePeerFile. Run install.sh again."
}
$durablePeer = (Get-Content -LiteralPath $durablePeerFile -Raw).Trim()
if ($durablePeer -ne $config.PeerId) {
    throw "The durable peer identity does not match PeerId in $configPath. Nothing was started."
}

$env:MULTIVERSE_EXPORT_EDGES = $config.ExportEdges
$env:MULTIVERSE_MIGRATION_EXCLUDE = $config.ExcludeSpecies
$env:MULTIVERSE_SIDECAR_PORT = $config.SidecarPort
# The sidecar mints this token at its first start, and the mod presents it on
# every Contract A connection. It is not the map credential.
$env:MULTIVERSE_CONTRACT_A_TOKEN_FILE = Join-Path $dataDir 'contract-a.token'
$env:MULTIVERSE_WORLD = $config.WorldName
$env:MULTIVERSE_SAVE_MINUTES = '10'
$env:MULTIVERSE_SAVE_KEEP = '6'
$env:MULTIVERSE_SAVE_ON_QUIT = 'true'
$env:MULTIVERSE_PORTAL = 'true'
$env:MULTIVERSE_PORTAL_FLOURISHES = 'true'
$env:MULTIVERSE_BROADCAST = 'true'
$env:MULTIVERSE_BROADCAST_ZOOM = '75'
$env:MULTIVERSE_BROADCAST_RESELECT_DELAY = '2'
$env:MULTIVERSE_BROADCAST_STATUS_FILE = Join-Path $state 'director.json'
$env:MULTIVERSE_BROADCAST_HIDE_UI = 'false'
$env:MULTIVERSE_BROADCAST_TIME_SCALE = '7.5'
$env:MULTIVERSE_BROADCAST_PANELS = 'brain,biology,biology'
$env:MULTIVERSE_BROADCAST_PANEL_SECONDS = '15'
$env:MULTIVERSE_BROADCAST_SHOW_FOV = 'true'
$env:MULTIVERSE_BROADCAST_DISABLE_SPAWN_TEMPLATES = 'Basic bibite'
$env:MULTIVERSE_MIN_FPS = 'off'
$env:MULTIVERSE_CMD_FILE = Join-Path $state 'command.txt'

# THE ORDER MATTERS. The sidecar mints the Contract A token the mod presents,
# and the map grants this world its place before the world exists to draw.
Remove-Item -LiteralPath $sidecarLog, "$sidecarLog.out" -Force -ErrorAction SilentlyContinue
$sidecarArguments = @(
    '--listen', "127.0.0.1:$($config.SidecarPort)",
    '--relay', $config.RelayUrl,
    '--peer-id', $config.PeerId,
    '--data-dir', $dataDir,
    '--credential-file', $credentialFile,
    '--log-file', $sidecarLog
)
$sidecar = Start-NativeProcess -FilePath $config.SidecarExe -WorkingDirectory $config.DataRoot `
    -ArgumentList $sidecarArguments -Hidden
Set-Content -LiteralPath (Join-Path $state 'sidecar.pid') -Value $sidecar.Id -NoNewline

# Everything below runs under one stop obligation. A sidecar left behind would
# hold this world's Contract B connection against the next attempt of the
# restart loop.
try {
    $granted = $null
    for ($attempt = 0; $attempt -lt 120; $attempt++) {
        $sidecar.Refresh()
        if ($sidecar.HasExited) { throw "The sidecar exited with code $($sidecar.ExitCode) before the map answered" }
        if (Test-Path -LiteralPath $sidecarLog) {
            $granted = Select-String -LiteralPath $sidecarLog -Pattern 'contract B: slot granted' -SimpleMatch |
                       Select-Object -Last 1
            if ($granted) { break }
            $refused = Select-String -LiteralPath $sidecarLog -SimpleMatch `
                           -Pattern 'placement claim refused', 'HTTP 401', 'certificate did not verify', 'below this relay' |
                       Select-Object -Last 1
            if ($refused) { throw "The map refused this world: $($refused.Line)" }
        }
        Start-Sleep -Seconds 1
    }
    if (-not $granted) {
        throw "The map did not grant this world a place within 120 seconds; read $sidecarLog"
    }
    Write-Host $granted.Line

    $gameExe = Join-Path $config.GameDir 'The Bibites.exe'
    if (-not (Test-Path -LiteralPath $gameExe)) { throw "Missing $gameExe" }
    # Keep this launch free of command-line arguments. On the current Windows host,
    # Start-Process with Unity arguments bypasses executable-local DLL redirection
    # and silently starts the game without BepInEx.
    $game = Start-Process -FilePath $gameExe -WorkingDirectory $config.GameDir `
        -PassThru
    Set-Content -LiteralPath (Join-Path $state 'game.pid') -Value $game.Id -NoNewline

    $handle = [IntPtr]::Zero
    for ($attempt = 0; $attempt -lt 180; $attempt++) {
        $game.Refresh()
        if ($game.HasExited) { throw "The Bibites exited with code $($game.ExitCode) before its window opened" }
        if ($game.MainWindowHandle -ne [IntPtr]::Zero) {
            $handle = $game.MainWindowHandle
            break
        }
        Start-Sleep -Seconds 1
    }
    if ($handle -eq [IntPtr]::Zero) { throw 'The Bibites window did not open' }

    $obsArguments = @(
        '--portable', '--multi', '--disable-updater', '--disable-missing-files-check',
        '--minimize-to-tray', '--startstreaming',
        '--profile', 'BibitesBroadcast', '--collection', 'BibitesBroadcast', '--scene', 'Broadcast'
    )
    $obs = Start-NativeProcess -FilePath $config.Obs -WorkingDirectory (Split-Path -Parent $config.Obs) `
        -ArgumentList $obsArguments
    Set-Content -LiteralPath (Join-Path $state 'obs.pid') -Value $obs.Id -NoNewline

    while ($true) {
        $game.Refresh()
        $obs.Refresh()
        $sidecar.Refresh()
        if ($game.HasExited) { throw "The Bibites exited with code $($game.ExitCode)" }
        if ($obs.HasExited) { throw "OBS exited with code $($obs.ExitCode)" }
        if ($sidecar.HasExited) { throw "The sidecar exited with code $($sidecar.ExitCode)" }
        Start-Sleep -Seconds 2
    }
} finally {
    & (Join-Path $root 'stop-windows.ps1')
}
