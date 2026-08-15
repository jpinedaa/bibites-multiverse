$ErrorActionPreference = 'SilentlyContinue'

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$state = Join-Path $root 'state'
$configPath = Join-Path $root 'config.env'
$gameDir = ''
if (Test-Path -LiteralPath $configPath) {
    foreach ($line in Get-Content -LiteralPath $configPath) {
        $pair = $line.Split('=', 2)
        if ($pair.Count -eq 2 -and $pair[0] -eq 'GameDir') { $gameDir = $pair[1] }
    }
}

function Get-RecordedProcess([string]$name) {
    $path = Join-Path $state "$name.pid"
    if (-not (Test-Path -LiteralPath $path)) { return $null }
    $text = Get-Content -LiteralPath $path -Raw
    $processId = 0
    if (-not [int]::TryParse($text, [ref]$processId)) { return $null }
    return Get-Process -Id $processId -ErrorAction SilentlyContinue
}

$obs = Get-RecordedProcess 'obs'
if ($obs -and $obs.Path -like '*\obs64.exe') {
    $obs.CloseMainWindow() | Out-Null
    if (-not $obs.WaitForExit(15000)) {
        Stop-Process -Id $obs.Id -Force -ErrorAction SilentlyContinue
    }
}

$game = Get-RecordedProcess 'game'
if ($game -and $gameDir -and $game.Path -like "$gameDir\*") {
    $game.CloseMainWindow() | Out-Null
    if (-not $game.WaitForExit(45000)) {
        Stop-Process -Id $game.Id -Force -ErrorAction SilentlyContinue
    }
}

Remove-Item -LiteralPath (Join-Path $state 'obs.pid') -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath (Join-Path $state 'game.pid') -Force -ErrorAction SilentlyContinue
