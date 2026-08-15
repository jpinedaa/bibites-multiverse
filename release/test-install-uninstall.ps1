<#
.SYNOPSIS
    Prove that the installer puts back exactly what it took, against a sandbox
    copy of the real Windows game.

.DESCRIPTION
    Runs the REAL Install-BibitesMultiverse.ps1 and Uninstall-BibitesMultiverse.ps1
    against throwaway copies of a real game under the temp directory, and
    compares each tree hash-for-hash before and after. It reads the source game
    but never changes it. It never touches a trust store, a running process or
    the network.

    Six scenarios:

      A  a machine with no BepInEx. The installer adds it; the game then writes
         BepInEx's config, log and cache; the uninstall must leave the tree
         byte-identical to before the install.
      B  a machine that already has BepInEx and another mod. The installer must
         touch neither, and the uninstall must remove only the multiverse plugin
         and its own settings file.
      C  a plugin somebody changed after the install. The uninstall must KEEP it
         and say so, rather than deleting a file it did not leave there.
      D  a game build that is not in the support matrix. The installer must
         refuse with INS-GAMEBUILD and create nothing at all.
      E  a private map, given -CaFile. Runs only when one is passed, and always
         with -SkipCaImport: this test never writes to a trust store.
      F  a complete package. The same installer creates a versioned managed
         runtime without -GameDir, and removes only unchanged payload files.

.PARAMETER KitDir
    The staged archive contents - the folder holding Install-BibitesMultiverse.ps1,
    the plugin, the sidecar, the BepInEx archive and support-matrix.json.
    Defaults to this script's own directory.

.PARAMETER RealGameDir
    An installed Windows game whose SHA-256 is in support-matrix.json. Each
    positive scenario uses a complete copy of this game. Existing BepInEx files
    are excluded from the clean copy.

.EXAMPLE
    .\test-install-uninstall.ps1 -KitDir .\kit -RealGameDir 'C:\Program Files (x86)\Steam\steamapps\common\The Bibites'
#>
[CmdletBinding()]
param(
    [string]$KitDir = '',
    [Parameter(Mandatory = $true)][string]$RealGameDir,
    [string]$CaFile = '',
    [switch]$KeepSandbox
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

if (-not $KitDir) { $KitDir = Split-Path -Parent $MyInvocation.MyCommand.Path }
$KitDir     = (Resolve-Path $KitDir).Path
$RealGameDir = (Resolve-Path $RealGameDir).Path
$realGameExe = Join-Path $RealGameDir 'The Bibites.exe'
$GameAssembly = Join-Path $RealGameDir 'The Bibites_Data\Managed\BibitesAssembly.dll'
foreach ($p in @($realGameExe, $GameAssembly)) {
    if (-not (Test-Path -LiteralPath $p -PathType Leaf)) { throw "not a real Windows game: $p" }
}
$launcher     = Join-Path $KitDir 'Install-BibitesMultiverse.cmd'
$guiInstaller = Join-Path $KitDir 'Install-BibitesMultiverse-Gui.ps1'
$gameFinder   = Join-Path $KitDir 'Find-BibitesGame.ps1'
$installer    = Join-Path $KitDir 'Install-BibitesMultiverse.ps1'
$uninstaller  = Join-Path $KitDir 'Uninstall-BibitesMultiverse.ps1'
foreach ($p in @($launcher, $guiInstaller, $gameFinder, $installer, $uninstaller)) {
    if (-not (Test-Path $p)) { throw "not in -KitDir: $p" }
}

$sandbox = Join-Path $env:TEMP ("bibites-multiverse-test-" + [guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Force -Path $sandbox | Out-Null

$script:failures = 0
$script:checks   = 0

function Check {
    param([string]$What, [bool]$Ok, [string]$Detail = '')
    $script:checks++
    if ($Ok) {
        Write-Host ("  PASS  {0}" -f $What) -ForegroundColor Green
    } else {
        $script:failures++
        Write-Host ("  FAIL  {0}" -f $What) -ForegroundColor Red
        if ($Detail) { Write-Host ("        {0}" -f $Detail) -ForegroundColor Red }
    }
}

function Scenario { param([string]$Name) Write-Host ""; Write-Host "==== $Name" -ForegroundColor Cyan }

$launcherText = Get-Content -LiteralPath $launcher -Raw
Check "the click launcher starts the PowerShell installer" `
    ($launcherText -match 'Install-BibitesMultiverse-Gui\.ps1')
Check "the click launcher uses process-only RemoteSigned" `
    ($launcherText -match '(?i)-ExecutionPolicy RemoteSigned')
Check "the click launcher never uses an execution-policy bypass" `
    (-not ($launcherText -match '(?i)ExecutionPolicy Bypass'))
$guiText = Get-Content -LiteralPath $guiInstaller -Raw
Check "the GUI selects start-after-install by default" `
    ($guiText -match '\$startAfter\.Checked\s*=\s*\$true')
Check "the GUI offers the included portable game" `
    ($guiText -match 'Use the included portable game')
Check "the GUI offers an existing game" `
    ($guiText -match 'Use a game that is already installed')
$probe = (& $guiInstaller -Probe | Out-String) | ConvertFrom-Json
Check "the game search finds a real installed game" `
    (Test-Path -LiteralPath (Join-Path ([string]$probe.foundGame) 'The Bibites.exe') -PathType Leaf)
$expectedRuntime = if (Test-Path -LiteralPath (Join-Path $KitDir 'game-payload.json') -PathType Leaf) {
    'bundled'
} else {
    'external'
}
Check "the GUI default matches the package edition" ($probe.defaultRuntime -eq $expectedRuntime)

function Get-TreeSnapshot {
    param([string]$Root)
    $snapshot = @{}
    if (-not (Test-Path $Root)) { return $snapshot }
    foreach ($file in @(Get-ChildItem -LiteralPath $Root -Recurse -File -Force)) {
        $rel = $file.FullName.Substring($Root.Length).TrimStart('\')
        $snapshot[$rel] = (Get-FileHash -Path $file.FullName -Algorithm SHA256).Hash
    }
    return $snapshot
}

function Compare-Snapshot {
    param($Before, $After)
    $problems = New-Object System.Collections.ArrayList
    foreach ($key in $Before.Keys) {
        if (-not $After.ContainsKey($key)) { [void]$problems.Add("REMOVED and should not have been: $key") }
        elseif ($After[$key] -ne $Before[$key]) { [void]$problems.Add("CHANGED: $key") }
    }
    foreach ($key in $After.Keys) {
        if (-not $Before.ContainsKey($key)) { [void]$problems.Add("LEFT BEHIND: $key") }
    }
    return $problems
}

function New-SandboxGame {
    param([string]$Path)
    New-Item -ItemType Directory -Force -Path $Path | Out-Null
    $modOverlay = @('BepInEx', 'winhttp.dll', 'version.dll', 'doorstop_config.ini', '.doorstop_version')
    Get-ChildItem -LiteralPath $RealGameDir -Force |
        Where-Object { $_.Name -notin $modOverlay } |
        Copy-Item -Destination $Path -Recurse -Force
    if (-not (Test-Path -LiteralPath (Join-Path $Path 'The Bibites.exe') -PathType Leaf)) {
        throw 'the real game copy has no The Bibites.exe'
    }
}

function New-JoinFile {
    param([string]$Path)
    $secret = -join ((1..64) | ForEach-Object { '0123456789abcdef'[(Get-Random -Maximum 16)] })
    Set-Content -Path $Path `
        -Value "multiverse-join/1 wss://relay.example.test/contract-b/v4 test-world.$secret" `
        -Encoding ASCII
    return $secret
}

function Invoke-Script {
    param([string]$Path, [hashtable]$Arguments)
    $global:LASTEXITCODE = 0
    $output = & $Path @Arguments *>&1 | Out-String
    return [pscustomobject]@{ Output = $output; ExitCode = $LASTEXITCODE }
}

Write-Host "sandbox: $sandbox"
Write-Host "kit    : $KitDir"

# ---------------------------------------------------------------- A

Scenario "A - a machine with no BepInEx"

$aRoot = Join-Path $sandbox 'A'
$aGame = Join-Path $aRoot 'game'
$aData = Join-Path $aRoot 'data'
New-SandboxGame -Path $aGame
$aJoin = Join-Path $aRoot 'join.txt'
$aSecret = New-JoinFile $aJoin

$beforeA = Get-TreeSnapshot $aGame

$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $aGame; DataRoot = $aData
    JoinStringFile = $aJoin; World = 'TestWorld'
}
Check "the installer succeeded" ($install.ExitCode -eq 0) $install.Output
Check "it states the all-four-edges default in its own output" `
    ($install.Output -match 'EXPORTS ON ALL FOUR EDGES')
Check "it states that nothing was imported into a trust store" `
    ($install.Output -match 'NOTHING WAS IMPORTED INTO ANY TRUST STORE')
Check "it never prints an execution-policy bypass" `
    (-not ($install.Output -match '(?i)executionpolicy\s+bypass'))
Check "it never tells anybody to pass an insecure flag" `
    (-not ($install.Output -match '(?i)--insecure-[a-z-]*\b(?![^\n]*never|[^\n]*no document)'))
Check "the plugin is in BepInEx\plugins" (Test-Path (Join-Path $aGame 'BepInEx\plugins\BibitesMultiverse.dll'))
Check "BepInEx was installed" (Test-Path (Join-Path $aGame 'BepInEx\core\BepInEx.dll'))
Check "the start script was written" (Test-Path (Join-Path $KitDir 'Start-Multiverse.ps1'))
Check "the stop script was written" (Test-Path (Join-Path $KitDir 'Stop-Multiverse.ps1'))

$recordPath = Join-Path $aData 'install-record.json'
Check "the install record exists" (Test-Path $recordPath)
if (Test-Path $recordPath) {
    $record = Get-Content -Raw -LiteralPath $recordPath | ConvertFrom-Json
    Check "the record says BepInEx was installed by the installer" ($record.bepInEx.installedByThisInstaller -eq $true)
    Check "the record says no certificate was imported" ($record.certificate.imported -eq $false)
}

$credential = Join-Path $aData 'peer-secret.txt'
Check "the credential file exists" (Test-Path $credential)
if (Test-Path $credential) {
    Check "the credential file holds the secret half only" `
        ((Get-Content -Raw -LiteralPath $credential).Trim() -eq $aSecret)
    Check "the credential file does not hold the identity half" `
        (-not ((Get-Content -Raw -LiteralPath $credential) -match 'test-world'))
}

# The game runs: BepInEx writes its own configuration, log and cache. None of
# these three is in the BepInEx archive - they appear on the first start - so
# the uninstall has to account for files no manifest ever listed.
New-Item -ItemType Directory -Force -Path (Join-Path $aGame 'BepInEx\cache') | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $aGame 'BepInEx\config') | Out-Null
Set-Content -Path (Join-Path $aGame 'BepInEx\config\BepInEx.cfg') -Value '[Logging]' -Encoding ASCII
Set-Content -Path (Join-Path $aGame 'BepInEx\config\dev.multiverse.bibites.cfg') -Value '[M4]' -Encoding ASCII
Set-Content -Path (Join-Path $aGame 'BepInEx\LogOutput.log') -Value 'a log' -Encoding ASCII
Set-Content -Path (Join-Path $aGame 'BepInEx\cache\chainloader.dat') -Value 'cache' -Encoding ASCII

# The generated start script: it has to parse, and it has to set all five
# settings explicitly - including the ones that match the mod's own default,
# which is the whole point of writing them out (Decision 7).
$startScript = Join-Path $KitDir 'Start-Multiverse.ps1'
if (Test-Path $startScript) {
    $errors = $null
    $tokens = $null
    [void][System.Management.Automation.Language.Parser]::ParseInput(
        (Get-Content -Raw -LiteralPath $startScript), [ref]$tokens, [ref]$errors)
    Check "the generated start script parses" (@($errors).Count -eq 0) (($errors | Out-String))
    $startText = Get-Content -Raw -LiteralPath $startScript
    foreach ($setting in @('MULTIVERSE_EXPORT_EDGES      = ''E,N,W,S''',
                           'MULTIVERSE_MIGRATION_EXCLUDE = ''Basic bibite''',
                           'MULTIVERSE_SAVE_MINUTES      = ''10''',
                           'MULTIVERSE_SAVE_KEEP         = ''6''',
                           'MULTIVERSE_SAVE_ON_QUIT      = ''true''')) {
        Check ("the start script sets " + $setting.Split('=')[0].Trim() + " explicitly") `
            ($startText.Contains('$env:' + $setting))
    }
    Check "the start script carries no secret" (-not ($startText -match $aSecret))
    Check "the start script passes the credential as a file, never as a value" `
        ($startText -match "--credential-file")
}
$stopScript = Join-Path $KitDir 'Stop-Multiverse.ps1'
if (Test-Path $stopScript) {
    $errors = $null
    $tokens = $null
    [void][System.Management.Automation.Language.Parser]::ParseInput(
        (Get-Content -Raw -LiteralPath $stopScript), [ref]$tokens, [ref]$errors)
    Check "the generated stop script parses" (@($errors).Count -eq 0) (($errors | Out-String))
    $stopText = Get-Content -Raw -LiteralPath $stopScript
    Check "the stop script stops only this install's recorded processes" `
        (-not ($stopText -match 'Stop-Process\s+-Name'))
}

$dry = Invoke-Script $uninstaller @{ DataRoot = $aData; DryRun = $true }
Check "the dry run succeeded" ($dry.ExitCode -eq 0) $dry.Output
Check "the dry run changed nothing" (Test-Path (Join-Path $aGame 'BepInEx\plugins\BibitesMultiverse.dll'))

$uninstall = Invoke-Script $uninstaller @{ DataRoot = $aData }
Check "the uninstall succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output

$afterA = Get-TreeSnapshot $aGame
$problems = @(Compare-Snapshot $beforeA $afterA)
Check "the game tree is hash-for-hash what it was before the install" ($problems.Count -eq 0) ($problems -join '; ')
Check "no BepInEx directory is left behind" (-not (Test-Path (Join-Path $aGame 'BepInEx')))
Check "the credential is gone" (-not (Test-Path $credential))
Check "the install record is gone" (-not (Test-Path $recordPath))
Check "the journal is kept, because nobody asked for it to go" (Test-Path (Join-Path $aData 'data'))
Check "the start script is gone" (-not (Test-Path (Join-Path $KitDir 'Start-Multiverse.ps1')))
Check "the stop script is gone" (-not (Test-Path (Join-Path $KitDir 'Stop-Multiverse.ps1')))

# ---------------------------------------------------------------- B

Scenario "B - a machine that already has BepInEx and another mod"

$bRoot = Join-Path $sandbox 'B'
$bGame = Join-Path $bRoot 'game'
$bData = Join-Path $bRoot 'data'
New-SandboxGame -Path $bGame
New-Item -ItemType Directory -Force -Path (Join-Path $bGame 'BepInEx\core') | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $bGame 'BepInEx\plugins') | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $bGame 'BepInEx\config') | Out-Null
Set-Content -Path (Join-Path $bGame 'BepInEx\core\BepInEx.dll') -Value 'somebody elses BepInEx' -Encoding ASCII
Set-Content -Path (Join-Path $bGame 'BepInEx\plugins\SomeOtherMod.dll') -Value 'another mod' -Encoding ASCII
Set-Content -Path (Join-Path $bGame 'BepInEx\config\BepInEx.cfg') -Value '[Logging]' -Encoding ASCII
Set-Content -Path (Join-Path $bGame 'winhttp.dll') -Value 'doorstop' -Encoding ASCII

$bJoin = Join-Path $bRoot 'join.txt'
[void](New-JoinFile $bJoin)
$beforeB = Get-TreeSnapshot $bGame

$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $bGame; DataRoot = $bData; JoinStringFile = $bJoin
}
Check "the installer succeeded" ($install.ExitCode -eq 0) $install.Output
Check "it left the existing BepInEx alone" ($install.Output -match 'already installed here; left exactly as it is')
$recordB = Get-Content -Raw -LiteralPath (Join-Path $bData 'install-record.json') | ConvertFrom-Json
Check "the record says BepInEx was not the installer's" ($recordB.bepInEx.installedByThisInstaller -eq $false)

Set-Content -Path (Join-Path $bGame 'BepInEx\config\dev.multiverse.bibites.cfg') -Value '[M4]' -Encoding ASCII

$uninstall = Invoke-Script $uninstaller @{ DataRoot = $bData }
Check "the uninstall succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
$afterB = Get-TreeSnapshot $bGame
$problems = @(Compare-Snapshot $beforeB $afterB)
Check "the game tree is hash-for-hash what it was before the install" ($problems.Count -eq 0) ($problems -join '; ')
Check "the other mod's plugin is untouched" (Test-Path (Join-Path $bGame 'BepInEx\plugins\SomeOtherMod.dll'))
Check "the existing BepInEx is untouched" (Test-Path (Join-Path $bGame 'BepInEx\core\BepInEx.dll'))

# ---------------------------------------------------------------- C

Scenario "C - a plugin somebody changed after the install"

$cRoot = Join-Path $sandbox 'C'
$cGame = Join-Path $cRoot 'game'
$cData = Join-Path $cRoot 'data'
New-SandboxGame -Path $cGame
$cJoin = Join-Path $cRoot 'join.txt'
[void](New-JoinFile $cJoin)

$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $cGame; DataRoot = $cData; JoinStringFile = $cJoin
}
Check "the installer succeeded" ($install.ExitCode -eq 0) $install.Output

$plugin = Join-Path $cGame 'BepInEx\plugins\BibitesMultiverse.dll'
Set-Content -Path $plugin -Value 'somebody replaced this by hand' -Encoding ASCII

$uninstall = Invoke-Script $uninstaller @{ DataRoot = $cData }
Check "the uninstall succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
Check "the changed plugin was KEPT rather than deleted" (Test-Path $plugin)
Check "the uninstall said so" ($uninstall.Output -match 'CHANGED since the install')

# ---------------------------------------------------------------- D

Scenario "D - a game build that is not in the support matrix"

$dRoot = Join-Path $sandbox 'D'
$dGame = Join-Path $dRoot 'game'
$dData = Join-Path $dRoot 'data'
New-SandboxGame -Path $dGame
$managed = Join-Path $dGame 'The Bibites_Data\Managed'
Set-Content -Path (Join-Path $managed 'BibitesAssembly.dll') -Value 'a different game build' -Encoding ASCII
$dJoin = Join-Path $dRoot 'join.txt'
[void](New-JoinFile $dJoin)

$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $dGame; DataRoot = $dData; JoinStringFile = $dJoin
}
Check "the installer refused" ($install.ExitCode -eq 1)
Check "it refused with INS-GAMEBUILD" ($install.Output -match 'INS-GAMEBUILD')
Check "it quoted the matrix's own refusal" ($install.Output -match 'This release supports one game build')
Check "it said nothing was installed" ($install.Output -match 'NOTHING was installed')
Check "no BepInEx was installed" (-not (Test-Path (Join-Path $dGame 'BepInEx')))
Check "no credential was written" (-not (Test-Path (Join-Path $dData 'peer-secret.txt')))
Check "no start script was written" (-not (Test-Path (Join-Path $KitDir 'Start-Multiverse.ps1')))

# ---------------------------------------------------------------- E

if ($CaFile) {
    Scenario "E - a private map, whose relay signs its own certificate"

    $eRoot = Join-Path $sandbox 'E'
    $eGame = Join-Path $eRoot 'game'
    $eData = Join-Path $eRoot 'data'
    New-SandboxGame -Path $eGame
    $eJoin = Join-Path $eRoot 'join.txt'
    [void](New-JoinFile $eJoin)

    # -SkipCaImport on purpose: this test never writes to a trust store, so the
    # branch is exercised up to the import and no further.
    $install = Invoke-Script $installer @{
        RuntimeSelection = 'external'; GameDir = $eGame; DataRoot = $eData; JoinStringFile = $eJoin
        CaFile = (Resolve-Path $CaFile).Path; SkipCaImport = $true
    }
    Check "the installer succeeded" ($install.ExitCode -eq 0) $install.Output
    Check "it printed what trusting an authority means" ($install.Output -match 'WHAT YOU ARE AGREEING TO')
    Check "it printed the thumbprint" ($install.Output -match 'thumbprint :')
    Check "it imported nothing, because -SkipCaImport" ($install.Output -match 'nothing was imported')
    Check "it kept a copy of the authority beside the data" (Test-Path (Join-Path $eData 'relay-ca.crt'))
    $recordE = Get-Content -Raw -LiteralPath (Join-Path $eData 'install-record.json') | ConvertFrom-Json
    Check "the record says nothing was imported" ($recordE.certificate.imported -eq $false)
    Check "the record still carries the thumbprint" ($recordE.certificate.thumbprint.Length -gt 0)

    $uninstall = Invoke-Script $uninstaller @{ DataRoot = $eData }
    Check "the uninstall succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
    Check "it left the trust store alone, because it never wrote to it" `
        ($uninstall.Output -match 'imported nothing into any trust store')
    Check "the copy of the authority is gone" (-not (Test-Path (Join-Path $eData 'relay-ca.crt')))
}

# ---------------------------------------------------------------- F

Scenario "F - a complete package with a bundled game payload"

$fRoot = Join-Path $sandbox 'F'
$fKit  = Join-Path $fRoot 'kit'
$fData = Join-Path $fRoot 'data'
New-Item -ItemType Directory -Force -Path $fKit | Out-Null
Get-ChildItem -LiteralPath $KitDir -Force | Copy-Item -Destination $fKit -Recurse -Force
Remove-Item -LiteralPath (Join-Path $fKit 'Start-Multiverse.ps1'), `
                         (Join-Path $fKit 'Stop-Multiverse.ps1') -Force -ErrorAction SilentlyContinue
$fPayload = Join-Path $fKit 'game'
New-SandboxGame -Path $fPayload
$fSha = (Get-FileHash -LiteralPath $GameAssembly -Algorithm SHA256).Hash.ToUpperInvariant()
Set-Content -LiteralPath (Join-Path $fKit 'GAME-REDISTRIBUTION-NOTICE.txt') -Value 'test redistribution permission' -Encoding ASCII
$descriptor = [ordered]@{
    format = 'bibites-multiverse/game-payload/1'
    platform = 'Windows'
    gameVersion = 'test'
    assemblySha256 = $fSha
    redistributionNoticeFile = 'GAME-REDISTRIBUTION-NOTICE.txt'
}
$descriptor | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $fKit 'game-payload.json') -Encoding ASCII
$manifestPath = Join-Path $fKit 'MANIFEST.sha256'
$manifestLines = @(Get-ChildItem -LiteralPath $fKit -File -Force -Recurse |
    Where-Object { $_.FullName -ne $manifestPath } |
    Sort-Object FullName |
    ForEach-Object {
        $relative = ($_.FullName.Substring($fKit.Length) -replace '^[\\/]+', '') -replace '\\', '/'
        '{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant(), $relative
    })
Set-Content -LiteralPath $manifestPath -Value $manifestLines -Encoding ASCII
$fJoin = Join-Path $fRoot 'join.txt'
[void](New-JoinFile $fJoin)
$fInstaller = Join-Path $fKit 'Install-BibitesMultiverse.ps1'
$fUninstaller = Join-Path $fKit 'Uninstall-BibitesMultiverse.ps1'
$fProbe = (& (Join-Path $fKit 'Install-BibitesMultiverse-Gui.ps1') -Probe | Out-String) | ConvertFrom-Json
Check "the complete package defaults to its included portable game" `
    ($fProbe.hasBundledGame -eq $true -and $fProbe.defaultRuntime -eq 'bundled')

$install = Invoke-Script $fInstaller @{ GameDir = $fPayload; DataRoot = $fData; JoinStringFile = $fJoin }
Check "the complete edition refuses an external game path" ($install.ExitCode -eq 1) $install.Output
Check "that refusal has the INS-RUNTIME taxonomy id" ($install.Output -match 'INS-RUNTIME')
Check "the refused selection copied no managed runtime" `
    (-not (Test-Path -LiteralPath (Join-Path $fData 'runtimes')))

$install = Invoke-Script $fInstaller @{ DataRoot = $fData; JoinStringFile = $fJoin }
Check "the complete installer succeeded without -GameDir" ($install.ExitCode -eq 0) $install.Output
Check "it selected the complete edition" ($install.Output -match 'complete edition: installed')
$fRuntime = Join-Path (Join-Path $fData 'runtimes') $fSha
Check "the game was copied into the versioned managed runtime" `
    (Test-Path -LiteralPath (Join-Path $fRuntime 'The Bibites.exe'))
$recordF = Get-Content -Raw -LiteralPath (Join-Path $fData 'install-record.json') | ConvertFrom-Json
Check "the record identifies a bundled managed runtime" `
    ($recordF.runtime.mode -eq 'bundled' -and $recordF.runtime.managedByThisInstaller)
$startF = Get-Content -Raw -LiteralPath (Join-Path $fKit 'Start-Multiverse.ps1')
Check "the generated start script points at the managed runtime" ($startF.Contains($fRuntime))

Set-Content -LiteralPath (Join-Path $fRuntime 'user-note.txt') -Value 'keep me' -Encoding ASCII
$uninstall = Invoke-Script $fUninstaller @{ DataRoot = $fData }
Check "the complete uninstall succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
Check "the unchanged game executable was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fRuntime 'The Bibites.exe')))
Check "a user-added runtime file was kept" (Test-Path -LiteralPath (Join-Path $fRuntime 'user-note.txt'))
Check "the uninstall explains why the non-empty runtime stays" ($uninstall.Output -match 'not empty, so it stays')

# ---------------------------------------------------------------- the verdict

Write-Host ""
if ($script:failures -eq 0) {
    Write-Host ("ALL {0} CHECKS PASSED" -f $script:checks) -ForegroundColor Green
} else {
    Write-Host ("{0} of {1} CHECKS FAILED" -f $script:failures, $script:checks) -ForegroundColor Red
}

if ($KeepSandbox) {
    Write-Host "sandbox kept at $sandbox"
} else {
    Remove-Item -LiteralPath $sandbox -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "sandbox removed"
}

if ($script:failures -gt 0) { exit 1 }
exit 0
