<#
.SYNOPSIS
    Prove that the installer puts back exactly what it took, against a sandbox
    game directory.

.DESCRIPTION
    Runs the REAL Install-BibitesMultiverse.ps1 and Uninstall-BibitesMultiverse.ps1
    against a throwaway game tree under the temp directory, and compares the tree
    hash-for-hash before and after. It never touches a Steam copy of the game, a
    trust store, a running process or the network.

    Four scenarios:

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

.PARAMETER KitDir
    The staged archive contents - the folder holding Install-BibitesMultiverse.ps1,
    the plugin, the sidecar, the BepInEx archive and support-matrix.json.
    Defaults to this script's own directory.

.PARAMETER GameAssembly
    A copy of the BibitesAssembly.dll whose SHA-256 is in support-matrix.json.
    The sandbox game is built around it, so that the matrix check passes for the
    same reason it passes on a real machine.

.EXAMPLE
    .\test-install-uninstall.ps1 -KitDir .\kit -GameAssembly .\BibitesAssembly.dll
#>
[CmdletBinding()]
param(
    [string]$KitDir = '',
    [Parameter(Mandatory = $true)][string]$GameAssembly,
    [string]$CaFile = '',
    [switch]$KeepSandbox
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

if (-not $KitDir) { $KitDir = Split-Path -Parent $MyInvocation.MyCommand.Path }
$KitDir       = (Resolve-Path $KitDir).Path
$GameAssembly = (Resolve-Path $GameAssembly).Path
$installer    = Join-Path $KitDir 'Install-BibitesMultiverse.ps1'
$uninstaller  = Join-Path $KitDir 'Uninstall-BibitesMultiverse.ps1'
foreach ($p in @($installer, $uninstaller)) {
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
    param([string]$Path, [string]$Assembly)
    $managed = Join-Path $Path 'The Bibites_Data\Managed'
    New-Item -ItemType Directory -Force -Path $managed | Out-Null
    Set-Content -Path (Join-Path $Path 'The Bibites.exe') -Value 'not a real game' -Encoding ASCII
    Copy-Item -Path $Assembly -Destination (Join-Path $managed 'BibitesAssembly.dll') -Force
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
New-SandboxGame -Path $aGame -Assembly $GameAssembly
$aJoin = Join-Path $aRoot 'join.txt'
$aSecret = New-JoinFile $aJoin

$beforeA = Get-TreeSnapshot $aGame

$install = Invoke-Script $installer @{
    GameDir = $aGame; DataRoot = $aData; JoinStringFile = $aJoin; World = 'TestWorld'
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
    [void][System.Management.Automation.Language.Parser]::ParseFile($startScript, [ref]$null, [ref]$errors)
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
New-SandboxGame -Path $bGame -Assembly $GameAssembly
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

$install = Invoke-Script $installer @{ GameDir = $bGame; DataRoot = $bData; JoinStringFile = $bJoin }
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
New-SandboxGame -Path $cGame -Assembly $GameAssembly
$cJoin = Join-Path $cRoot 'join.txt'
[void](New-JoinFile $cJoin)

$install = Invoke-Script $installer @{ GameDir = $cGame; DataRoot = $cData; JoinStringFile = $cJoin }
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
$managed = Join-Path $dGame 'The Bibites_Data\Managed'
New-Item -ItemType Directory -Force -Path $managed | Out-Null
Set-Content -Path (Join-Path $dGame 'The Bibites.exe') -Value 'not a real game' -Encoding ASCII
Set-Content -Path (Join-Path $managed 'BibitesAssembly.dll') -Value 'a different game build' -Encoding ASCII
$dJoin = Join-Path $dRoot 'join.txt'
[void](New-JoinFile $dJoin)

$install = Invoke-Script $installer @{ GameDir = $dGame; DataRoot = $dData; JoinStringFile = $dJoin }
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
    New-SandboxGame -Path $eGame -Assembly $GameAssembly
    $eJoin = Join-Path $eRoot 'join.txt'
    [void](New-JoinFile $eJoin)

    # -SkipCaImport on purpose: this test never writes to a trust store, so the
    # branch is exercised up to the import and no further.
    $install = Invoke-Script $installer @{
        GameDir = $eGame; DataRoot = $eData; JoinStringFile = $eJoin
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
