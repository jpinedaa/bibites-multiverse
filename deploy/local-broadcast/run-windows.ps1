$ErrorActionPreference = 'Stop'

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
foreach ($name in @('GameDir', 'Obs', 'WorldName', 'PublishPort')) {
    if (-not $config.ContainsKey($name) -or -not $config[$name]) { throw "Missing $name in $configPath" }
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

$env:MULTIVERSE_EXPORT_EDGES = 'none'
$env:MULTIVERSE_WORLD = $config.WorldName
$env:MULTIVERSE_SAVE_MINUTES = '10'
$env:MULTIVERSE_SAVE_KEEP = '6'
$env:MULTIVERSE_SAVE_ON_QUIT = 'true'
$env:MULTIVERSE_PORTAL = 'false'
$env:MULTIVERSE_PORTAL_FLOURISHES = 'false'
$env:MULTIVERSE_BROADCAST = 'true'
$env:MULTIVERSE_BROADCAST_ZOOM = '35'
$env:MULTIVERSE_BROADCAST_RESELECT_DELAY = '2'
$env:MULTIVERSE_BROADCAST_STATUS_FILE = Join-Path $state 'director.json'
$env:MULTIVERSE_BROADCAST_HIDE_UI = 'true'
$env:MULTIVERSE_MIN_FPS = 'off'
$env:MULTIVERSE_CMD_FILE = Join-Path $state 'command.txt'

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
$obs = Start-Process -FilePath $config.Obs -WorkingDirectory (Split-Path -Parent $config.Obs) `
    -ArgumentList $obsArguments -PassThru
Set-Content -LiteralPath (Join-Path $state 'obs.pid') -Value $obs.Id -NoNewline

try {
    while ($true) {
        $game.Refresh()
        $obs.Refresh()
        if ($game.HasExited) { throw "The Bibites exited with code $($game.ExitCode)" }
        if ($obs.HasExited) { throw "OBS exited with code $($obs.ExitCode)" }
        Start-Sleep -Seconds 2
    }
} finally {
    & (Join-Path $root 'stop-windows.ps1')
}
