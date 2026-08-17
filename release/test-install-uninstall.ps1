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

    Seven scenarios:

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
      G  a world that is already in the data root. An uninstall keeps its
         credential; installing again over it - with the same join string, or
         with none at all - keeps the same identity, spends no second place on
         the map and never rewrites the secret file. A secret is replaced only
         for the world the install record itself names, and the replaced one is
         kept beside it. A DIFFERENT identity - which is what a slot handover
         mints - takes -ReplaceWorldIdentity, and is refused without it; so is a
         secret nothing can name, a claim only an ordinary text file makes, a
         file that is not a credential, and a -RelayUrl pointed at another map. A
         credential behind a blank first line is still a credential. THE SIDECAR
         LOG is read as the last witness: one identity in it is adopted with the
         relay from the same line, two are refused and listed, and a kit
         unpacked beside the data root counts as a place to look. A secret in a
         data root NO sidecar ever ran in is an orphan of an interrupted
         install: it is renamed aside, kept, and the install goes on.
         -RemoveWorldData is the one path that ends the world.

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
# A machine may run a second copy of the game under another account, or
# elevated, and this account cannot read that process's path. Neither script may
# refuse on that fact alone: what decides it is whether the files in the folder
# being written to are held open.
$installerText   = Get-Content -LiteralPath $installer -Raw
$uninstallerText = Get-Content -LiteralPath $uninstaller -Raw
foreach ($pair in @(@{ name = 'installer'; text = $installerText },
                    @{ name = 'uninstall'; text = $uninstallerText })) {
    Check ("the $($pair.name) probes the folder's own files for a lock") `
        ($pair.text -match 'function Test-FileLocked')
    Check ("the $($pair.name) asks for ReadWrite with no sharing, which only a running copy refuses") `
        ($pair.text -match '\[System\.IO\.FileShare\]::None')
    Check ("the $($pair.name) never treats a process it cannot inspect as running here on its own") `
        (-not ($pair.text -match 'if \(-not \$exe\) \{ (return \$true|\[void\]\$hit\.Add)'))
}
$guiText = Get-Content -LiteralPath $guiInstaller -Raw
Check "the GUI selects start-after-install by default" `
    ($guiText -match '\$startAfter\.Checked\s*=\s*\$true')
Check "the GUI offers the included portable game" `
    ($guiText -match 'Use the included portable game')
Check "the GUI offers an existing game" `
    ($guiText -match 'Use a game that is already installed')
# START-AFTER-INSTALL IS WHY THESE THREE ARE HERE. `Start-Process -Wait` waits on
# a job object that is not empty until the child AND every descendant has ended,
# and the default install ends by starting a sidecar and a game that are meant to
# outlive it - so the window never came back, and the setup around it never wrote
# the uninstaller, the shortcuts or the Uninstall registry key. The behaviour is
# measured in release/test-installer-wait.ps1; this is the source-level guard,
# and it reads code lines only so the comment that explains it cannot satisfy it.
$guiCode = ((Get-Content -LiteralPath $guiInstaller) | Where-Object { $_ -notmatch '^\s*#' }) -join "`n"
Check "the GUI never starts a process with -Wait, which waits on the descendant tree" `
    (-not ($guiCode -match 'Start-Process[^\r\n]*\s-Wait\b'))
Check "the GUI waits on the process object it started" `
    ($guiCode -match '\.WaitForExit\(\)')
Check "the GUI keeps the handle its exit code is read through" `
    ($guiCode -match '\$process\.Handle')

# NOTHING OF THIS INSTALL MAY BE RUNNING WHILE IT IS BEING REPLACED. Windows
# holds a program's own file open for as long as it runs, so a re-install started
# over a live world died in step 9's Copy-Item with a raw
# "The process cannot access the file" - AFTER steps 1 to 8 had already replaced
# the mod inside the game and settled this world's map identity, and after the
# setup around it had given up on writing the shortcuts and the Installed apps
# entry. A refusal that arrives then is worth almost nothing, so the question is
# asked FIRST, before a single byte is written, and it names what to close.
#
# These read CODE LINES ONLY, like the three above: the comments that explain the
# check must not be able to satisfy it.
$installerCode = ((Get-Content -LiteralPath $installer) | Where-Object { $_ -notmatch '^\s*#' }) -join "`n"
$stepZeroAt   = $installerCode.IndexOf('Step "0 of 9')
$stepOneAt    = $installerCode.IndexOf('Step "1 of 9')
$firstWriteAt = $installerCode.IndexOf('Unblock-File')
Check "the installer asks what is running in a step 0 of its own" ($stepZeroAt -ge 0)
Check "step 0 comes before step 1" `
    ($stepZeroAt -ge 0 -and $stepOneAt -gt $stepZeroAt) "step 0 at $stepZeroAt, step 1 at $stepOneAt"
Check "step 0 comes before the first thing this installer writes anywhere" `
    ($stepZeroAt -ge 0 -and $firstWriteAt -gt $stepZeroAt) `
    "step 0 at $stepZeroAt, first write at $firstWriteAt"
Check "step 0 ends in the refusal rather than in a warning" `
    ($installerCode -match '(?s)Step "0 of 9.*?if \(\$busy\.Count -gt 0\) \{ Stop-Busy')
Check "step 0 asks Win32_Process, so a game launched from elsewhere is not flagged" `
    ($installerCode -match 'Get-CimInstance -ClassName Win32_Process')
Check "step 0 asks about the windowed launcher, the console launcher and the sidecar" `
    ($installerCode -match 'foreach \(\$program in @\(\$LauncherName, \$ConsoleLauncherName, \$SidecarName\)\)')
Check "the console launcher is asked about by its own name" `
    ($installerCode -match '\$ConsoleLauncherName = ''multiverse-launcher\.exe''')
Check "step 0 asks about the application folder only when this run installs into one" `
    ($installerCode -match '\[string\]::IsNullOrWhiteSpace\(\$InstallRoot\)\) \{ \$plannedProgramRoot = Get-FullPath \$InstallRoot \}')
Check "step 0 asks about the game in the folder step 5 writes the mod into" `
    ($installerCode -match 'Get-BusyProcess ''The Bibites\.exe'' \$plannedGameRoot')
# Step 0 and step 2 must agree about which game this run installs against. A
# step 0 that guessed 'bundled' where step 2 goes external would refuse an
# install over a game it was never going to touch, so ONE function decides.
Check "one rule decides which game this run installs against, and both steps use it" `
    (([regex]::Matches($installerCode, 'Resolve-RuntimeSelection \$RuntimeSelection \$GameDir \$hasBundledPayload')).Count -ge 2)
Check "the installer no longer chooses the runtime a second time by hand" `
    (-not ($installerCode -match 'elseif \(\$hasBundledPayload\) \{'))
Check "the installer counts a process it cannot inspect as opaque, never as running here" `
    ($installerCode -match 'if \(-not \$row\.image\) \{ \$opaque\+\+; continue \}')
Check "a folder is matched with its own separator, so a sibling folder is not it" `
    ($installerCode -match '\$prefix = \$full\.TrimEnd' -and $installerCode -match 'DirectorySeparatorChar')
# NOTHING IS EVER ENDED FOR THE PERSON RUNNING SETUP. A world ended rather than
# stopped loses everything it has simulated since its last save, so step 0 may
# name a process and may never end one. The scope is step 0 itself - its constant,
# its functions and its own body - because the stop script this installer WRITES
# ends processes for a living, and that is the whole point of it.
$stepZeroRegion = ''
$busyBlockAt = $installerCode.IndexOf('$ExitBusy = 3')
if ($busyBlockAt -ge 0 -and $stepOneAt -gt $busyBlockAt) {
    $stepZeroRegion = $installerCode.Substring($busyBlockAt, $stepOneAt - $busyBlockAt)
}
Check "step 0's own code can be read whole" ($stepZeroRegion -ne '')
foreach ($weapon in @('Stop-Process', '(?i)taskkill', '\.Kill\(')) {
    Check ("step 0 never ends a process for you: $weapon") `
        ($stepZeroRegion -ne '' -and -not ($stepZeroRegion -match $weapon))
}
# The refusal itself, read whole, because every line of it is the remedy.
$busyRefusal = ''
$busyMatch = [regex]::Match($installerText, '(?s)function Stop-Busy \{.*?\r?\n\}')
if ($busyMatch.Success) { $busyRefusal = $busyMatch.Value }
Check "the running-programs refusal is one place in the installer" ($busyRefusal -ne '')
Check "it carries its own taxonomy id" ($busyRefusal -match 'INS-BUSY')
Check "it names this install's own stop script" ($busyRefusal -match '\$StopName')
Check "that stop script is Stop-Multiverse.ps1" `
    ($installerCode -match '\$StopName\s*=\s*''Stop-Multiverse\.ps1''')
Check "it names the launcher's own stop of every world" ($busyRefusal -match 'stop --all')
Check "it names the launcher window's own way to do that" ($busyRefusal -match 'Stop every world')
Check "it says that nothing was changed" ($busyRefusal -match 'NOTHING WAS CHANGED')
Check "it says that it ended nothing itself" ($busyRefusal -match 'NOTHING IS ENDED FOR YOU')
Check "it says what an install that got as far as step 9 had already done" `
    ($busyRefusal -match 'WHAT THIS INSTALL HAD ALREADY DONE')
# The exit code is the one thing a caller can act on without reading a word.
Check "the refusal has an exit code of its own" ($installerCode -match '\$ExitBusy = 3')
Check "the refusal exits with it" ($busyRefusal -match 'exit \$ExitBusy')
Check "no other refusal in the installer uses that code" `
    (([regex]::Matches($installerCode, 'exit 3(\s|$)')).Count -eq 0)
Check "the GUI knows the same code" ($guiCode -match '\$ExitBusy = 3')
Check "the GUI gives that code a dialog of its own" ($guiCode -match 'if \(\$exitCode -eq \$ExitBusy\)')
# Step 9 replaces the launcher and the sidecar, and something can start between
# step 0 and step 9 - a clicked shortcut, a started world. That copy says the
# same thing rather than printing a sharing violation with a line number in it.
Check "step 9's program copy names a sharing violation instead of showing it raw" `
    ($installerCode -match 'Copy-ProgramFile \$source \$destination')
# -cnotmatch, because Copy-ProgramFile's own body copies $Source to $Destination
# and PowerShell's -match would call that the very line it is looking for.
Check "step 9 has no raw copy of a program file left" `
    ($installerCode -cnotmatch 'Copy-Item -LiteralPath \$source -Destination \$destination')
Check "that copy re-throws every failure that is not a lock" `
    ($installerCode -match '(?s)function Copy-ProgramFile.*?\n    \} catch \{.*?\n        throw \$failure')
$probe = (& $guiInstaller -Probe | Out-String) | ConvertFrom-Json
Check "the game search finds a real installed game" `
    (Test-Path -LiteralPath (Join-Path ([string]$probe.foundGame) 'The Bibites.exe') -PathType Leaf)
$expectedRuntime = if (Test-Path -LiteralPath (Join-Path $KitDir 'game-payload.json') -PathType Leaf) {
    'bundled'
} else {
    'external'
}
Check "the GUI default matches the package edition" ($probe.defaultRuntime -eq $expectedRuntime)
Check "the GUI probe verifies the package manifest" `
    ($probe.manifestMatches -eq $true -and $probe.manifestFiles -gt 0)

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

# The launcher is the application's entry point, and the profile is what it
# reads. The profile states this same world in the launcher's own format, and
# it must never carry the credential: that stays in peer-secret.txt.
Check "the launcher is in the kit" (Test-Path (Join-Path $KitDir 'BibitesMultiverseLauncher.exe'))
# The window and its commands are two files, and step 9 refuses to install with
# either one missing (INS-CHECKSUM) whenever it is managing an application
# directory - which every setup and every -InstallRoot install is. A kit with one
# of them is therefore a kit that cannot be installed the way anybody installs.
Check "the launcher's commands are in the kit" (Test-Path (Join-Path $KitDir 'multiverse-launcher.exe'))
$profilesDir        = Join-Path $KitDir 'profiles'
$defaultProfilePath = Join-Path $profilesDir 'default.json'
$activeProfilePath  = Join-Path $profilesDir 'active.txt'
Check "the launcher's default profile was written" (Test-Path $defaultProfilePath)
if (Test-Path $defaultProfilePath) {
    $profileText = Get-Content -Raw -LiteralPath $defaultProfilePath
    $profileObj  = $profileText | ConvertFrom-Json
    Check "the profile states the launcher-profile format" `
        ($profileObj.format -eq 'bibites-multiverse/launcher-profile/1')
    $profileKeys = @($profileObj.PSObject.Properties.Name)
    foreach ($key in @('format', 'name', 'gameDir', 'dataRoot', 'sidecarPort', 'world',
                       'headless', 'exportEdges', 'excludeSpecies', 'saveMinutes', 'saveKeep',
                       'saveOnQuit', 'peerId', 'relayUrl', 'createdUtc')) {
        Check ("the profile carries $key") ($profileKeys -contains $key) ($profileKeys -join ', ')
    }
    Check "the profile carries nothing else" ($profileKeys.Count -eq 15) ($profileKeys -join ', ')
    Check "the profile is the world this install was given" `
        ($profileObj.name -eq 'default' -and $profileObj.world -eq 'TestWorld')
    Check "the profile carries this install's port, game and data root" `
        ($profileObj.sidecarPort -eq 8787 -and $profileObj.gameDir -eq $aGame -and
         $profileObj.dataRoot -eq $aData) `
        ("$($profileObj.sidecarPort) / $($profileObj.gameDir) / $($profileObj.dataRoot)")
    Check "the profile carries the identity this install was given" `
        ($profileObj.peerId -eq 'test-world' -and
         $profileObj.relayUrl -eq 'wss://relay.example.test/contract-b/v4')
    Check "the profile runs with a picture unless somebody asks otherwise" `
        ($profileObj.headless -eq $false)
    Check "the profile carries the settings this install ships with" `
        ($profileObj.exportEdges -eq 'E,N,W,S' -and $profileObj.excludeSpecies -eq 'Basic bibite' -and
         $profileObj.saveMinutes -eq 10 -and $profileObj.saveKeep -eq 6 -and
         $profileObj.saveOnQuit -eq $true)
    Check "the profile carries no secret" (-not ($profileText -match $aSecret))
}
Check "the launcher's selected world is the one the installer wrote" `
    ((Test-Path $activeProfilePath) -and
     ((Get-Content -Raw -LiteralPath $activeProfilePath).Trim() -eq 'default'))

$recordPath = Join-Path $aData 'install-record.json'
Check "the install record exists" (Test-Path $recordPath)
if (Test-Path $recordPath) {
    $record = Get-Content -Raw -LiteralPath $recordPath | ConvertFrom-Json
    Check "the record says BepInEx was installed by the installer" ($record.bepInEx.installedByThisInstaller -eq $true)
    Check "the record says no certificate was imported" ($record.certificate.imported -eq $false)
    Check "the record is the revision that names the profiles" `
        ($record.record -eq 'bibites-multiverse/install-record/3') ([string]$record.record)
    Check "the record carries this world and its port" `
        ($record.world -eq 'TestWorld' -and $record.sidecarPort -eq 8787)
    Check "the record carries the settings this install shipped with" `
        ($record.settings.exportEdges -eq 'E,N,W,S' -and
         $record.settings.excludeSpecies -eq 'Basic bibite' -and
         $record.settings.saveMinutes -eq 10 -and $record.settings.saveKeep -eq 6 -and
         $record.settings.saveOnQuit -eq 'true')
    Check "the record points at the launcher's profiles" `
        ($record.profiles.root -eq $profilesDir -and $record.profiles.default -eq 'default')
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

# The generated start script: it has to parse, and it has to set every one of
# its settings explicitly - including the ones that match the mod's own default,
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
                           'MULTIVERSE_SAVE_ON_QUIT      = ''true''',
                           'MULTIVERSE_STARTUP_TIME_SCALE = ''10''')) {
        Check ("the start script sets " + $setting.Split('=')[0].Trim() + " explicitly") `
            ($startText.Contains('$env:' + $setting))
    }
    # THE SILENT-FAILURE GATE. A start script that does not check whether the mod
    # reached the sidecar is a start script that reports success for a world
    # sitting at the main menu (LOCAL-CONFIGRACE).
    foreach ($probe in @('/my-slot',
                         'THE GAME STARTED BUT ITS MOD HAS NOT REACHED THE SIDECAR',
                         'LOCAL-CONFIGRACE',
                         'LOCAL-STARVATION',
                         'the game joined the map: mod connected',
                         'this is a warning, not a failure')) {
        Check ("the start script checks that the mod connected (" + $probe + ")") `
            ($startText.Contains($probe))
    }
    # THE LOSSLESS-STOP GATE. A headless game has no window, so there is no close
    # request to post to it; the mod's command file is the only ask it can hear,
    # and the mod reads its path once, at start. A start script that does not set
    # it is a world that can only ever be killed (LOCAL-HEADLESSSTOP).
    Check "the start script names this world's mod command file" `
        ($startText.Contains('$env:MULTIVERSE_CMD_FILE = Join-Path $DataRoot ''cmd.txt'''))
    Check "the start script clears a command an interrupted stop left behind" `
        ($startText.Contains('Remove-Item -LiteralPath $env:MULTIVERSE_CMD_FILE'))
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
    # Asking first is what lets the world's save-on-quit run; the force is the
    # fallback for a process that does not answer. The three checks below are the
    # difference between "asked and confirmed" and "asked and hoped": a process
    # with no window makes taskkill refuse, and only Get-Process can say whether
    # anything actually stopped.
    Check "the stop script asks the world to close before it forces it" `
        ($stopText -match 'taskkill')
    Check "the stop script reads what taskkill answered, so a refusal forces at once" `
        ($stopText -match '\$LASTEXITCODE')
    Check "the stop script confirms the process is gone with Get-Process" `
        ($stopText -match 'Get-Process -Id \$processId')
    Check "the stop script never trusts WaitForExit for a process it did not start" `
        (-not ($stopText -match 'WaitForExit'))
    # THE ASK MUST NOT CARRY /T, and the FORCE must. /T walks the process tree and
    # refuses the whole call when any member of it needs /F - and the game always
    # spawns a windowless UnityCrashHandler64.exe, so an ask with /T never reached
    # the game and every stop skipped save-on-quit.
    Check "the stop script asks with taskkill and no /T" `
        ($stopText.Contains('& taskkill.exe /PID $processId *> $null'))
    Check "the stop script forces with the process tree, which takes the crash handler" `
        ($stopText.Contains('& taskkill.exe /PID $processId /T /F'))
    Check "the stop script never asks with /T" `
        (-not ($stopText -match 'taskkill\.exe /PID \$processId /T \*'))
    # A headless world is asked through its mod, which is the only ask it hears.
    foreach ($probe in @('function Request-ModQuit',
                         'Move-Item -LiteralPath $temporary -Destination $CmdFile -Force',
                         'nothing is reading $CmdFile',
                         'MULTIVERSE_CMD_FILE was set',
                         'it is saving and shutting down')) {
        Check ("the stop script asks the mod to quit first (" + $probe + ")") `
            ($stopText.Contains($probe))
    }
    Check "the stop script passes the game's command file to the stop" `
        ($stopText.Contains("Stop-Recorded (Join-Path `$DataRoot 'game.pid') 'the game' 30 (Join-Path `$DataRoot 'cmd.txt')"))
    Check "the stop script keeps the pid file when it could not stop the process" `
        ($stopText -match 'COULD NOT STOP')
}

$dry = Invoke-Script $uninstaller @{ DataRoot = $aData; DryRun = $true }
Check "the dry run succeeded" ($dry.ExitCode -eq 0) $dry.Output
Check "the dry run changed nothing" (Test-Path (Join-Path $aGame 'BepInEx\plugins\BibitesMultiverse.dll'))
Check "the dry run says the profiles directory goes, rather than that it stays" `
    (-not ($dry.Output -match ("not empty, so it stays : " + [regex]::Escape($profilesDir)))) $dry.Output

# The profiles directory is ordinary user-writable JSON in an ordinary
# user-writable folder, and -RemoveWorldData deletes recursively. A file this
# script did not write must never steer that: not a foreign format, not a name
# that disagrees with its file name, and never a data root like a drive root.
$rogueProfiles = [ordered]@{
    'rogue-root.json' = ('{"format":"bibites-multiverse/launcher-profile/1","name":"rogue-root",' +
                         '"gameDir":"","dataRoot":"C:\\"}')
    'rogue-name.json' = ('{"format":"bibites-multiverse/launcher-profile/1","name":"somethingelse",' +
                         '"gameDir":"","dataRoot":"C:\\Rogue"}')
    'rogue-fmt.json'  = ('{"format":"something/else/1","name":"rogue-fmt",' +
                         '"gameDir":"","dataRoot":"C:\\Rogue"}')
}
foreach ($rogueName in $rogueProfiles.Keys) {
    Set-Content -LiteralPath (Join-Path $profilesDir $rogueName) `
        -Value $rogueProfiles[$rogueName] -Encoding ASCII
}
$rogue = Invoke-Script $uninstaller @{ DataRoot = $aData; RemoveWorldData = $true; DryRun = $true }
Check "the rogue dry run succeeded" ($rogue.ExitCode -eq 0) $rogue.Output
Check "no rogue profile's data root is ever named for removal" `
    (-not ($rogue.Output -match '(?i)C:\\(Rogue|data|logs)')) $rogue.Output
foreach ($rogueName in $rogueProfiles.Keys) {
    Check ("the rogue profile $rogueName is kept and the ledger says so") `
        ($rogue.Output -match ('stays : [^\r\n]*' + [regex]::Escape($rogueName))) $rogue.Output
    Check ("the rogue profile $rogueName is still on disk") `
        (Test-Path -LiteralPath (Join-Path $profilesDir $rogueName))
    Remove-Item -LiteralPath (Join-Path $profilesDir $rogueName) -Force
}

$uninstall = Invoke-Script $uninstaller @{ DataRoot = $aData }
Check "the uninstall succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output

$afterA = Get-TreeSnapshot $aGame
$problems = @(Compare-Snapshot $beforeA $afterA)
Check "the game tree is hash-for-hash what it was before the install" ($problems.Count -eq 0) ($problems -join '; ')
Check "no BepInEx directory is left behind" (-not (Test-Path (Join-Path $aGame 'BepInEx')))
Check "the credential is kept, because the world it names is still on the map" `
    (Test-Path $credential)
Check "the ledger says the identity stays" ($uninstall.Output -match 'keeps its place on the map') $uninstall.Output
Check "the install record is gone" (-not (Test-Path $recordPath))
Check "the journal is kept, because nobody asked for it to go" (Test-Path (Join-Path $aData 'data'))
Check "the start script is gone" (-not (Test-Path (Join-Path $KitDir 'Start-Multiverse.ps1')))
Check "the stop script is gone" (-not (Test-Path (Join-Path $KitDir 'Stop-Multiverse.ps1')))
Check "the launcher's profiles directory is gone" (-not (Test-Path $profilesDir))

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
Check "no profiles directory was created" (-not (Test-Path (Join-Path $KitDir 'profiles')))

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
$fProgram = Join-Path $fRoot 'program'
$fExternalGame = Join-Path $fRoot 'external-game'
$fExternalData = Join-Path $fRoot 'external-data'
$fExternalProgram = Join-Path $fRoot 'external-program'
New-Item -ItemType Directory -Force -Path $fKit | Out-Null
Get-ChildItem -LiteralPath $KitDir -Force | Copy-Item -Destination $fKit -Recurse -Force
Remove-Item -LiteralPath (Join-Path $fKit 'Start-Multiverse.ps1'), `
                         (Join-Path $fKit 'Stop-Multiverse.ps1') -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath (Join-Path $fKit 'profiles') -Recurse -Force -ErrorAction SilentlyContinue
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
$fProbe = (& (Join-Path $fKit 'Install-BibitesMultiverse-Gui.ps1') -Probe | Out-String) | ConvertFrom-Json
Check "the complete package defaults to its included portable game" `
    ($fProbe.hasBundledGame -eq $true -and $fProbe.defaultRuntime -eq 'bundled')
Check "the complete package probe verifies every embedded file" `
    ($fProbe.manifestMatches -eq $true -and $fProbe.manifestFiles -gt 0)

New-SandboxGame -Path $fExternalGame
$fExternalBefore = Get-TreeSnapshot $fExternalGame
$install = Invoke-Script $fInstaller @{
    GameDir = $fExternalGame; DataRoot = $fExternalData
    InstallRoot = $fExternalProgram; JoinStringFile = $fJoin
}
Check "the complete installer accepts an existing game" ($install.ExitCode -eq 0) $install.Output
Check "it did not copy the included portable game" `
    ($install.Output -match 'included portable game will not be copied')
$recordExternal = Get-Content -Raw -LiteralPath `
    (Join-Path $fExternalData 'install-record.json') | ConvertFrom-Json
Check "the existing-game record is external" `
    ($recordExternal.runtime.mode -eq 'external' -and -not $recordExternal.runtime.managedByThisInstaller)
Check "the existing-game choice created no managed runtime" `
    (-not (Test-Path -LiteralPath (Join-Path $fExternalData 'runtimes')))
$uninstall = Invoke-Script (Join-Path $fExternalProgram 'Uninstall-BibitesMultiverse.ps1') `
    @{ DataRoot = $fExternalData }
Check "the existing-game uninstall succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
$fExternalAfter = Get-TreeSnapshot $fExternalGame
$fExternalDiff = @(Compare-Snapshot $fExternalBefore $fExternalAfter)
Check "the existing game is unchanged after uninstall" ($fExternalDiff.Count -eq 0) `
    ($fExternalDiff -join '; ')

$install = Invoke-Script $fInstaller @{
    DataRoot = $fData; InstallRoot = $fProgram; JoinStringFile = $fJoin
}
Check "the complete installer succeeded without -GameDir" ($install.ExitCode -eq 0) $install.Output
Check "it selected the complete edition" ($install.Output -match 'complete edition: installed')
$fRuntime = Join-Path (Join-Path $fData 'runtimes') $fSha
Check "the game was copied into the versioned managed runtime" `
    (Test-Path -LiteralPath (Join-Path $fRuntime 'The Bibites.exe'))
$recordF = Get-Content -Raw -LiteralPath (Join-Path $fData 'install-record.json') | ConvertFrom-Json
Check "the record identifies a bundled managed runtime" `
    ($recordF.runtime.mode -eq 'bundled' -and $recordF.runtime.managedByThisInstaller)
Check "the record identifies the installed application directory" `
    ($recordF.program.root -eq $fProgram)
Check "the application directory contains the launcher icon" `
    (Test-Path -LiteralPath (Join-Path $fProgram 'bibites-multiverse.ico'))
Check "the application directory contains the sidecar" `
    (Test-Path -LiteralPath (Join-Path $fProgram 'multiverse-sidecar.exe'))
Check "the application directory contains the launcher" `
    (Test-Path -LiteralPath (Join-Path $fProgram 'BibitesMultiverseLauncher.exe'))
Check "the application directory contains the launcher's commands" `
    (Test-Path -LiteralPath (Join-Path $fProgram 'multiverse-launcher.exe'))
Check "the application directory contains the map the launcher enrolls new worlds with" `
    (Test-Path -LiteralPath (Join-Path $fProgram 'public-map.json'))
Check "the application directory contains the launcher's default profile" `
    (Test-Path -LiteralPath (Join-Path $fProgram 'profiles\default.json'))
$startF = Get-Content -Raw -LiteralPath (Join-Path $fProgram 'Start-Multiverse.ps1')
Check "the generated start script points at the managed runtime" ($startF.Contains($fRuntime))

Set-Content -LiteralPath (Join-Path $fRuntime 'user-note.txt') -Value 'keep me' -Encoding ASCII
$fUninstaller = Join-Path $fProgram 'Uninstall-BibitesMultiverse.ps1'
$uninstall = Invoke-Script $fUninstaller @{ DataRoot = $fData }
Check "the complete uninstall succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
Check "the unchanged game executable was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fRuntime 'The Bibites.exe')))
Check "a user-added runtime file was kept" (Test-Path -LiteralPath (Join-Path $fRuntime 'user-note.txt'))
Check "the uninstall explains why the non-empty runtime stays" ($uninstall.Output -match 'not empty, so it stays')
Check "the installed sidecar was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'multiverse-sidecar.exe')))
Check "the installed launcher was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'BibitesMultiverseLauncher.exe')))
Check "the installed launcher's commands were removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'multiverse-launcher.exe')))
Check "the installed public map was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'public-map.json')))
Check "the launcher's profiles directory was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'profiles')))

# ---------------------------------------------------------------- G

Scenario "G - a world that is already in the data root"

$gRoot    = Join-Path $sandbox 'G'
$gGame    = Join-Path $gRoot 'game'
$gData    = Join-Path $gRoot 'data'
$gProgram = Join-Path $gRoot 'program'
New-SandboxGame -Path $gGame
$gJoin   = Join-Path $gRoot 'join.txt'
$gSecret = New-JoinFile $gJoin

$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gData
    InstallRoot = $gProgram; JoinStringFile = $gJoin; World = 'TestWorld'
}
Check "the first install succeeded" ($install.ExitCode -eq 0) $install.Output
$gCredential = Join-Path $gData 'peer-secret.txt'
$gRecordPath = Join-Path $gData 'install-record.json'
$gPeerIdFile = Join-Path $gData 'data\peer-id'
Check "the installer wrote the world's identity beside its journal" `
    ((Test-Path -LiteralPath $gPeerIdFile) -and
     ((Get-Content -Raw -LiteralPath $gPeerIdFile).Trim() -eq 'test-world'))

$gUninstaller = Join-Path $gProgram 'Uninstall-BibitesMultiverse.ps1'
$uninstall = Invoke-Script $gUninstaller @{ DataRoot = $gData }
Check "the uninstall succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
Check "it keeps the world's credential" (Test-Path -LiteralPath $gCredential)
Check "it keeps the identity beside the journal" (Test-Path -LiteralPath $gPeerIdFile)
Check "it removes its own record" (-not (Test-Path -LiteralPath $gRecordPath))

# Installing again over an uninstalled world is that same world: the same
# identity, the same secret file, and no second place on the map.
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gData
    InstallRoot = $gProgram; JoinStringFile = $gJoin; World = 'TestWorld'
}
Check "installing again after an uninstall succeeded" ($install.ExitCode -eq 0) $install.Output
Check "it says the identity is the same world and only the secret changes" `
    ($install.Output -match 'the same world, test-world') $install.Output
$recordG = Get-Content -Raw -LiteralPath $gRecordPath | ConvertFrom-Json
Check "the new record names the same world" ($recordG.peerId -eq 'test-world')
$profileG = Get-Content -Raw -LiteralPath (Join-Path $gProgram 'profiles\default.json') | ConvertFrom-Json
Check "the launcher's profile names the same world" ($profileG.peerId -eq 'test-world')
Check "the credential is the secret that join string carries" `
    ((Get-Content -Raw -LiteralPath $gCredential).Trim() -eq $gSecret)

# The reported defect: no join string at all. An empty data root enrolls a new
# public identity here; a data root with a world in it must adopt that world,
# and must not reach the network to do it.
if (Test-Path -LiteralPath (Join-Path $KitDir 'public-map.json')) {
    $install = Invoke-Script $installer @{
        RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gData; InstallRoot = $gProgram
    }
    Check "a repair with no join string succeeded" ($install.ExitCode -eq 0) $install.Output
    Check "it reused the identity already in the data root" `
        ($install.Output -match 'reusing the map identity already in') $install.Output
    Check "it asked the map for nothing" `
        (-not ($install.Output -match 'requesting a unique identity')) $install.Output
    $recordG = Get-Content -Raw -LiteralPath $gRecordPath | ConvertFrom-Json
    Check "the repair kept the same world and its own relay" `
        ($recordG.peerId -eq 'test-world' -and
         $recordG.relayUrl -eq 'wss://relay.example.test/contract-b/v4')
    Check "the repair kept the secret byte for byte" `
        ((Get-Content -Raw -LiteralPath $gCredential).Trim() -eq $gSecret)
}

# A different identity over the same world's journal strands both.
$gOtherJoin = Join-Path $gRoot 'other-join.txt'
Set-Content -Path $gOtherJoin `
    -Value "multiverse-join/1 wss://relay.example.test/contract-b/v4 other-world.$gSecret" `
    -Encoding ASCII
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gData
    InstallRoot = $gProgram; JoinStringFile = $gOtherJoin
}
Check "a second identity over the same world is refused" ($install.ExitCode -eq 1) $install.Output
Check "that refusal carries INS-ENROLL" ($install.Output -match 'INS-ENROLL') $install.Output
Check "the refusal names the world that is here and the one offered" `
    (($install.Output -match 'test-world') -and ($install.Output -match 'other-world')) $install.Output
Check "the refusal left the credential alone" `
    ((Get-Content -Raw -LiteralPath $gCredential).Trim() -eq $gSecret)

# A secret with nothing left to name its world, in a folder a world HAS run in,
# is never overwritten. The journal is what says a world ran here: sidecar.New
# opens it before anything else, and this installer never writes inside data\.
$gOrphan = Join-Path $gRoot 'orphan'
New-Item -ItemType Directory -Force -Path (Join-Path $gOrphan 'data\journal') | Out-Null
$gOrphanSecret = ('a' * 64)
Set-Content -LiteralPath (Join-Path $gOrphan 'peer-secret.txt') -Value $gOrphanSecret -Encoding ASCII
Set-Content -LiteralPath (Join-Path $gOrphan 'data\journal\journal.log') -Value '{"seq":1}' -Encoding ASCII
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gOrphan
    InstallRoot = $gProgram; JoinStringFile = $gJoin
}
Check "a secret no file can name, where a world has run, is refused" ($install.ExitCode -eq 1) $install.Output
Check "that refusal carries INS-ENROLL" ($install.Output -match 'INS-ENROLL') $install.Output
Check "the refusal names the file it would have overwritten" `
    ($install.Output -match 'peer-secret\.txt') $install.Output
Check "the unnamed secret is exactly as it was" `
    ((Get-Content -Raw -LiteralPath (Join-Path $gOrphan 'peer-secret.txt')).Trim() -eq $gOrphanSecret)
Check "nothing was renamed aside" `
    (@(Get-ChildItem -LiteralPath $gOrphan -Filter 'peer-secret.txt.*.orphan' -File -ErrorAction SilentlyContinue).Count -eq 0)

# THE ORPHAN OF AN INTERRUPTED INSTALL: a secret in a data root no sidecar ever
# ran in belongs to no world, so it is renamed aside and the install goes on.
# This runs on the join-string path, so nothing here reaches a network; the
# enrolling half is proved on Linux against the suite's fake endpoint.
$gNeverRan = Join-Path $gRoot 'never-ran'
New-Item -ItemType Directory -Force -Path $gNeverRan | Out-Null
$gNeverRanSecret = ('e' * 64)
Set-Content -LiteralPath (Join-Path $gNeverRan 'peer-secret.txt') -Value $gNeverRanSecret -Encoding ASCII
$gNeverRanJoin = Join-Path $gRoot 'never-ran-join.txt'
$gNeverRanNew = -join ((1..64) | ForEach-Object { '0123456789abcdef'[(Get-Random -Maximum 16)] })
Set-Content -Path $gNeverRanJoin `
    -Value "multiverse-join/1 wss://relay.example.test/contract-b/v4 never-ran-world.$gNeverRanNew" `
    -Encoding ASCII
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gNeverRan
    InstallRoot = $gProgram; JoinStringFile = $gNeverRanJoin
}
Check "a secret no world ever used installs rather than refusing" ($install.ExitCode -eq 0) $install.Output
Check "it says the secret was an orphan" ($install.Output -match 'was an orphan') $install.Output
Check "it says why - the world never ran" ($install.Output -match 'stopped before') $install.Output
$gOrphans = @(Get-ChildItem -LiteralPath $gNeverRan -Filter 'peer-secret.txt.*.orphan' -File -ErrorAction SilentlyContinue)
Check "the orphaned secret is KEPT, not deleted" ($gOrphans.Count -eq 1)
if ($gOrphans.Count -eq 1) {
    Check "and it is the secret that was there" `
        ((Get-Content -Raw -LiteralPath $gOrphans[0].FullName).Trim() -ceq $gNeverRanSecret)
}
Check "the world is the one the join string names" `
    ((Get-Content -Raw -LiteralPath (Join-Path $gNeverRan 'install-record.json') | ConvertFrom-Json).peerId -ceq 'never-ran-world')
Check "and its secret is in place" `
    ((Get-Content -Raw -LiteralPath (Join-Path $gNeverRan 'peer-secret.txt')).Trim() -ceq $gNeverRanNew)

# The secret of a world the install record itself names may be replaced, and the
# one it replaces is kept, never destroyed.
$gHandoverSecret = -join ((1..64) | ForEach-Object { '0123456789abcdef'[(Get-Random -Maximum 16)] })
$gHandoverJoin = Join-Path $gRoot 'handover-join.txt'
Set-Content -Path $gHandoverJoin `
    -Value "multiverse-join/1 wss://relay.example.test/contract-b/v4 test-world.$gHandoverSecret" `
    -Encoding ASCII
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gData
    InstallRoot = $gProgram; JoinStringFile = $gHandoverJoin; World = 'TestWorld'
}
Check "a new secret for the world the record names is applied" ($install.ExitCode -eq 0) $install.Output
Check "it says it is the same world" ($install.Output -match 'the same world, test-world') $install.Output
Check "the new secret is in place" `
    ((Get-Content -Raw -LiteralPath $gCredential).Trim() -ceq $gHandoverSecret)
$gBackups = @(Get-ChildItem -LiteralPath $gData -Filter 'peer-secret.txt.*.old' -File -ErrorAction SilentlyContinue)
Check "the replaced secret is kept beside it rather than destroyed" ($gBackups.Count -eq 1)
if ($gBackups.Count -eq 1) {
    Check "and the kept copy is the secret it replaced" `
        ((Get-Content -Raw -LiteralPath $gBackups[0].FullName).Trim() -ceq $gSecret)
    Remove-Item -LiteralPath $gBackups[0].FullName -Force
}

# A slot handover mints a NEW identity (contract-b-m4.md §7.5), so this is both
# the credential-recovery path and the shape of a mistake. Gated either way.
$gHandoverJoinB = Join-Path $gRoot 'handover-b.txt'
$gHandoverSecretB = -join ((1..64) | ForEach-Object { '0123456789abcdef'[(Get-Random -Maximum 16)] })
Set-Content -Path $gHandoverJoinB `
    -Value "multiverse-join/1 wss://relay.example.test/contract-b/v4 test-world-b.$gHandoverSecretB" `
    -Encoding ASCII
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gData
    InstallRoot = $gProgram; JoinStringFile = $gHandoverJoinB
}
Check "a join string for another identity is refused" ($install.ExitCode -eq 1) $install.Output
Check "the refusal names both worlds" `
    (($install.Output -match 'test-world-b') -and ($install.Output -match 'is the world test-world')) $install.Output
Check "the refusal names the switch a handover needs" `
    ($install.Output -match 'ReplaceWorldIdentity') $install.Output
Check "the world it would have replaced still has its own secret" `
    ((Get-Content -Raw -LiteralPath $gCredential).Trim() -ceq $gHandoverSecret)

$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gData
    InstallRoot = $gProgram; JoinStringFile = $gHandoverJoinB; ReplaceWorldIdentity = $true
}
Check "the switch applies the handover" ($install.ExitCode -eq 0) $install.Output
$recordG = Get-Content -Raw -LiteralPath $gRecordPath | ConvertFrom-Json
Check "the new identity is the world's" ($recordG.peerId -ceq 'test-world-b')
Check "the name it used to answer to is kept for its operator" `
    ((Get-Content -Raw -LiteralPath (Join-Path $gData 'data\peer-id.previous')).Trim() -ceq 'test-world')
Check "the change of identity is stated at the end" `
    ($install.Output -match 'CHANGED IDENTITY') $install.Output
$gBackups = @(Get-ChildItem -LiteralPath $gData -Filter 'peer-secret.txt.*.old' -File -ErrorAction SilentlyContinue)
Check "the secret it replaced is kept too" ($gBackups.Count -eq 1)
foreach ($b in $gBackups) { Remove-Item -LiteralPath $b.FullName -Force }
$gSecret = $gHandoverSecretB

# A secret a hand-written claim names is NOT the same world proven: an ordinary
# text file may keep a world and may never destroy one.
$gUnproven = Join-Path $gRoot 'unproven'
New-Item -ItemType Directory -Force -Path (Join-Path $gUnproven 'data') | Out-Null
$gUnprovenSecret = ('y' * 64)
Set-Content -LiteralPath (Join-Path $gUnproven 'peer-secret.txt') -Value $gUnprovenSecret -Encoding ASCII
Set-Content -LiteralPath (Join-Path $gUnproven 'data\peer-id') -Value 'test-world' -Encoding ASCII
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gUnproven
    InstallRoot = $gProgram; JoinStringFile = $gJoin
}
Check "a join string over an unproven claim is refused" ($install.ExitCode -eq 1) $install.Output
Check "that refusal carries INS-ENROLL" ($install.Output -match 'INS-ENROLL') $install.Output
Check "the refusal names the file that made the claim" ($install.Output -match 'peer-id') $install.Output
Check "the refusal says what such a file may and may not do" `
    ($install.Output -match 'ordinary text file') $install.Output
Check "the secret it was about to destroy is still there" `
    ((Get-Content -Raw -LiteralPath (Join-Path $gUnproven 'peer-secret.txt')).Trim() -ceq $gUnprovenSecret)

# A credential whose FIRST LINE is blank is still a credential. Reading one line
# rather than the file would call it absent, delete it, and take a new identity.
$gBlank = Join-Path $gRoot 'blank-first-line'
New-Item -ItemType Directory -Force -Path (Join-Path $gBlank 'data') | Out-Null
$gBlankSecret = ('e' * 64)
Set-Content -LiteralPath (Join-Path $gBlank 'peer-secret.txt') `
    -Value ([Environment]::NewLine + $gBlankSecret) -Encoding ASCII
Set-Content -LiteralPath (Join-Path $gBlank 'data\peer-id') -Value 'test-world' -Encoding ASCII
Set-Content -LiteralPath (Join-Path $gBlank 'data\relay-url') `
    -Value 'wss://relay.example.test/contract-b/v4' -Encoding ASCII
$gBlankBefore = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $gBlank 'peer-secret.txt')).Hash
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gBlank; InstallRoot = $gProgram
}
Check "a credential behind a blank first line is adopted, not replaced" ($install.ExitCode -eq 0) $install.Output
Check "it says so" ($install.Output -match 'reusing the map identity already in') $install.Output
Check "the credential is byte-identical afterwards" `
    ((Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $gBlank 'peer-secret.txt')).Hash -eq $gBlankBefore)
$recordBlank = Get-Content -Raw -LiteralPath (Join-Path $gBlank 'install-record.json') | ConvertFrom-Json
Check "the record names the world that file belongs to" ($recordBlank.peerId -ceq 'test-world')
Check "and the map it says it is on" `
    ($recordBlank.relayUrl -ceq 'wss://relay.example.test/contract-b/v4')

# Something that is not a credential is a refusal, never an overwrite.
$gJunk = Join-Path $gRoot 'not-a-credential'
New-Item -ItemType Directory -Force -Path $gJunk | Out-Null
Set-Content -LiteralPath (Join-Path $gJunk 'peer-secret.txt') -Value 'hello' -Encoding ASCII
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gJunk
    InstallRoot = $gProgram; JoinStringFile = $gJoin
}
Check "a peer-secret.txt that is not a credential is refused" ($install.ExitCode -eq 1) $install.Output
Check "the refusal says what a credential is" ($install.Output -match 'not a map credential') $install.Output
Check "it changes that file not at all" `
    ((Get-Content -Raw -LiteralPath (Join-Path $gJunk 'peer-secret.txt')).Trim() -ceq 'hello')

# A name whose secret is gone, taking a DIFFERENT identity: a second place on the
# map unless the operator handed this world's slot over, and this machine cannot
# tell which. The public-map half of this gate is proved on Linux, against the
# test's fake enrollment endpoint; here it runs on the join-string path so that
# nothing in this file can reach a network.
$gLost = Join-Path $gRoot 'lost-secret'
New-Item -ItemType Directory -Force -Path (Join-Path $gLost 'data') | Out-Null
Set-Content -LiteralPath (Join-Path $gLost 'data\peer-id') -Value 'lost-world' -Encoding ASCII
Set-Content -LiteralPath (Join-Path $gLost 'data\relay-url') `
    -Value 'wss://relay.example.test/contract-b/v4' -Encoding ASCII
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gLost
    InstallRoot = $gProgram; JoinStringFile = $gJoin
}
Check "a new identity over a world whose secret is gone is refused" ($install.ExitCode -eq 1) $install.Output
Check "the refusal names the world that would go dark" ($install.Output -match 'lost-world') $install.Output
Check "the refusal names the switch that accepts the cost" `
    ($install.Output -match 'ReplaceWorldIdentity') $install.Output
Check "no credential was written" `
    (-not (Test-Path -LiteralPath (Join-Path $gLost 'peer-secret.txt')))

$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gLost
    InstallRoot = $gProgram; JoinStringFile = $gJoin; ReplaceWorldIdentity = $true
}
Check "-ReplaceWorldIdentity takes the new identity" ($install.ExitCode -eq 0) $install.Output
Check "the world that went dark is kept where the operator can be told" `
    ((Get-Content -Raw -LiteralPath (Join-Path $gLost 'data\peer-id.previous')).Trim() -ceq 'lost-world')
Check "the summary names the change of identity" ($install.Output -match 'CHANGED IDENTITY') $install.Output

# -RelayUrl names the map a world is on; it does not move one.
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gData
    InstallRoot = $gProgram; RelayUrl = 'wss://two.example.test/contract-b/v4'
}
Check "-RelayUrl at another map is refused" ($install.ExitCode -eq 1) $install.Output
Check "the refusal names both maps" `
    (($install.Output -match 'relay\.example\.test') -and ($install.Output -match 'two\.example\.test')) $install.Output

# THE SIDECAR'S OWN LOG is the last witness a data root keeps, and the state a
# pre-profiles kit unpacked somewhere else leaves behind. slog writes one
# key=value per attribute and the sidecar's logger carries peer= on every line.
function New-SidecarLog {
    param([string]$Path, [string]$Peer, [string]$Relay)
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Path) | Out-Null
    $lines = @(
        'time=2026-08-16T12:00:00.000Z level=WARN msg="sidecar: no peer credential configured"',
        ('time=2026-08-16T12:00:00.100Z level=INFO msg="sidecar: listening" peer=' + $Peer +
         ' addr=127.0.0.1:8787 path=/contract-a/v2 relay=' + $Relay +
         ' dataDir=x preferredSlot=0 relayCredential=configured'),
        ('time=2026-08-16T12:00:02.000Z level=INFO msg="contract B: slot granted" peer=' + $Peer +
         ' slot=3 position=1,0 reason=granted map=m slotCount=5 lanes="E->4 N->2"')
    )
    Add-Content -LiteralPath $Path -Value $lines -Encoding ASCII
}

# Adoption never reaches a network, so these run whenever the kit carries the
# packaged map; the enrolling half of this gate is proved on Linux against the
# suite's fake enrollment endpoint.
if (Test-Path -LiteralPath (Join-Path $KitDir 'public-map.json')) {
    $gLogOne = Join-Path $gRoot 'log-one-world'
    $gLogOnePeer = 'public-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
    $gLogOneSecret = ('a' * 64)
    New-Item -ItemType Directory -Force -Path $gLogOne | Out-Null
    Set-Content -LiteralPath (Join-Path $gLogOne 'peer-secret.txt') -Value $gLogOneSecret -Encoding ASCII
    New-SidecarLog (Join-Path $gLogOne 'logs\sidecar.log') $gLogOnePeer 'wss://relay.example.test/contract-b/v4'
    $install = Invoke-Script $installer @{
        RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gLogOne; InstallRoot = $gProgram
    }
    Check "one identity in the sidecar log is adopted" ($install.ExitCode -eq 0) $install.Output
    Check "it names the log line it read" `
        ($install.Output -match 'sidecar\.log \("sidecar: listening"\)') $install.Output
    Check "it never asked the map for an identity" `
        (-not ($install.Output -match 'requesting a unique identity')) $install.Output
    $recordLog = Get-Content -Raw -LiteralPath (Join-Path $gLogOne 'install-record.json') | ConvertFrom-Json
    Check "the record names the world the log named" ($recordLog.peerId -ceq $gLogOnePeer)
    Check "and the map that log line named" `
        ($recordLog.relayUrl -ceq 'wss://relay.example.test/contract-b/v4')
    Check "the credential is byte-identical afterwards" `
        ((Get-Content -Raw -LiteralPath (Join-Path $gLogOne 'peer-secret.txt')).Trim() -ceq $gLogOneSecret)
    Check "the name is written beside the journal, where a later install finds it first" `
        ((Get-Content -Raw -LiteralPath (Join-Path $gLogOne 'data\peer-id')).Trim() -ceq $gLogOnePeer)

    # Two worlds in the logs: the installer will not choose, and says which two.
    $gLogTwo = Join-Path $gRoot 'log-two-worlds'
    $gLogPeerA = 'public-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
    $gLogPeerB = 'public-cccccccccccccccccccccccccccccccc'
    $gLogTwoSecret = ('c' * 64)
    New-Item -ItemType Directory -Force -Path $gLogTwo | Out-Null
    Set-Content -LiteralPath (Join-Path $gLogTwo 'peer-secret.txt') -Value $gLogTwoSecret -Encoding ASCII
    New-SidecarLog (Join-Path $gLogTwo 'logs\sidecar.log.1') $gLogPeerA 'wss://relay.example.test/contract-b/v4'
    New-SidecarLog (Join-Path $gLogTwo 'logs\sidecar.log') $gLogPeerB 'wss://relay.example.test/contract-b/v4'
    $install = Invoke-Script $installer @{
        RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gLogTwo; InstallRoot = $gProgram
    }
    Check "two identities in the logs are refused" ($install.ExitCode -eq 1) $install.Output
    Check "that refusal carries INS-ENROLL" ($install.Output -match 'INS-ENROLL') $install.Output
    Check "the refusal says the installer read the log itself" `
        ($install.Output -match 'READ THE SIDECAR LOG') $install.Output
    Check "it lists both worlds" `
        (($install.Output -match $gLogPeerA) -and ($install.Output -match $gLogPeerB)) $install.Output
    Check "it changed nothing" `
        ((Get-Content -Raw -LiteralPath (Join-Path $gLogTwo 'peer-secret.txt')).Trim() -ceq $gLogTwoSecret)

    # The remedy it prints has to work, and it wins over the log.
    New-Item -ItemType Directory -Force -Path (Join-Path $gLogTwo 'data') | Out-Null
    Set-Content -LiteralPath (Join-Path $gLogTwo 'data\peer-id') -Value $gLogPeerB -Encoding ASCII
    $install = Invoke-Script $installer @{
        RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gLogTwo; InstallRoot = $gProgram
    }
    Check "naming one of them installs it" ($install.ExitCode -eq 0) $install.Output
    $recordLog = Get-Content -Raw -LiteralPath (Join-Path $gLogTwo 'install-record.json') | ConvertFrom-Json
    Check "the named world is the one in the record" ($recordLog.peerId -ceq $gLogPeerB)
}

# A kit unpacked BESIDE the data root, which is where an advanced ZIP goes. Its
# start script names this data root, and that is a stronger claim than a log.
$gBesideKit = Join-Path $gRoot 'beside-the-kit'
$gBesideData = Join-Path $gBesideKit 'data-root'
New-Item -ItemType Directory -Force -Path $gBesideData | Out-Null
Set-Content -LiteralPath (Join-Path $gBesideData 'peer-secret.txt') -Value ('d' * 64) -Encoding ASCII
Set-Content -LiteralPath (Join-Path $gBesideKit 'Start-Multiverse.ps1') -Encoding ASCII -Value @(
    "`$GameDir     = '$gGame'",
    "`$DataRoot    = '$gBesideData'",
    "`$RelayUrl    = 'wss://beside.example.test/contract-b/v4'",
    "`$PeerId      = 'priv-beside'"
)
$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $gGame; DataRoot = $gBesideData
    InstallRoot = $gProgram; RelayUrl = 'wss://beside.example.test/contract-b/v4'
}
Check "a kit unpacked beside the data root is found" ($install.ExitCode -eq 0) $install.Output
Check "it adopts the world that start script names" `
    ($install.Output -match 'peer priv-beside') $install.Output
$recordBeside = Get-Content -Raw -LiteralPath (Join-Path $gBesideData 'install-record.json') | ConvertFrom-Json
Check "on the map that start script names" `
    ($recordBeside.relayUrl -ceq 'wss://beside.example.test/contract-b/v4')

# The one path that does end a world says so, and takes the credential with it.
$uninstall = Invoke-Script $gUninstaller @{ DataRoot = $gData; RemoveWorldData = $true }
Check "the uninstall with -RemoveWorldData succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
Check "-RemoveWorldData removes the credential" (-not (Test-Path -LiteralPath $gCredential))
Check "-RemoveWorldData says the world ends on the map" `
    ($uninstall.Output -match 'end of this world on the map') $uninstall.Output

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
