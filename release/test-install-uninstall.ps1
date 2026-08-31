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

    Eight scenarios:

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
         Then the whole cycle that used to end in INS-RUNTIME: a world runs and
         BepInEx writes its log, its config and its cache; a second install goes
         over the same managed runtime and still records that framework as its
         own; the uninstall reclaims the managed game copy WHOLE, residue
         included, and keeps a ledger file of what it did; and a third install
         starts again from the payload. A husk built here by hand - every
         recorded game file gone, the framework left - is rebuilt by an install
         rather than refused. A game file somebody changed is still kept, and
         still keeps the folder around it.
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
      H  AN UPGRADE, which is the only thing the homepage download ever does on
         a machine that already has this - and it passes no settings at all. A
         world renamed, a port moved, a window turned off and a second world set
         as the one this installation opens on all survive it; so do the
         journal, the log and the credential. A file the release before this one
         shipped and this one does not is removed, and one somebody edited is
         kept and said so. A setting NAMED on the command line still wins over
         the kept one. THE TWO NAMES THIS WORLD IS PUBLISHED UNDER survive it as
         well, and so does a DECLINE - taken with '-', kept by a run that names
         nothing, and kept when the launcher's profile is gone and the install
         record is the only thing left that remembers the answer. The framework
         this package unpacked into a game folder somebody else chose is still
         this install's after every one of those, so the uninstall at the end
         takes it back out instead of leaving it there.

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
Check "the GUI says that the default option opens the launcher too" `
    ($guiText -match 'After installation, start The Bibites, connect, and open the launcher')
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
Check "the GUI locates the graphical launcher in the installed application directory" `
    ($guiCode -match '\$launcherPath\s*=\s*Join-Path\s+\$programRoot\s+''BibitesMultiverseLauncher\.exe''')
Check "the GUI opens the launcher when start-after-install was selected" `
    ($guiCode -match '(?s)if \(\$startAfter\.Checked\) \{.{0,1200}Start-Process -FilePath \$launcherPath -WorkingDirectory \$programRoot')
Check "a launcher-window failure does not report the completed install as failed" `
    ($guiCode -match '(?s)catch \{\s*\$launcherFailure = \$_\.Exception\.Message.{0,800}The launcher could not open')

# ------------------------------------ the two names, against the window's own code
#
# THE GRAPHICAL SETUP IS THE ONLY INSTALLER MOST PARTICIPANTS EVER RUN, and every
# question this project asks about the two published names it answers from its own
# two boxes. So what goes INTO those boxes, and what comes out of them on the way
# to the child process, is worth testing against the real functions rather than a
# copy of them - which is what -DefineOnly is for: it loads the readers, the
# bounds and the quoting without opening a window or writing a byte.
. $guiInstaller -DefineOnly

# WHAT THE BOXES ARE FILLED IN WITH. An upgrade must offer what this installation
# already publishes, because the window PASSES those boxes as -Keeper and
# -WorldName and a named flag beats the stored answer: a box filled from the
# Windows account name would rename a world that had one, and un-decline a
# decline. A fresh install is the only case the account name is offered in.
$nRoot    = Join-Path $sandbox 'names'
$nData    = Join-Path $nRoot 'data'
$nProgram = Join-Path $nRoot 'program'
New-Item -ItemType Directory -Force -Path (Join-Path $nProgram 'profiles') | Out-Null
New-Item -ItemType Directory -Force -Path $nData | Out-Null
$nProfilePath = Join-Path $nProgram 'profiles\default.json'
$nRecordPath  = Join-Path $nData 'install-record.json'

$nNone = Get-PreviousPublicNames -DataRoot $nData -ProgramRoot $nProgram
Check "a computer with no install here has answered neither name" `
    ((-not $nNone.Present) -and (-not $nNone.KeeperAnswered) -and (-not $nNone.WorldNameAnswered))

# The record alone, carrying a DECLINE. This is the case the installer's own
# answered-check used to get wrong: it read the launcher profile only, so a data
# root whose record said "publishes nothing" and whose profile was gone counted
# as never asked - and the account name was offered over somebody's refusal.
@{ record = 'bibites-multiverse/install-record/3'; dataRoot = $nData
   settings = @{ keeper = ''; worldName = '' } } |
    ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $nRecordPath -Encoding ASCII
$nRecordOnly = Get-PreviousPublicNames -DataRoot $nData -ProgramRoot $nProgram
Check "the install record alone is enough to have been asked" `
    ($nRecordOnly.Present -and $nRecordOnly.KeeperAnswered -and $nRecordOnly.WorldNameAnswered)
Check "and a decline in it fills both boxes with nothing" `
    ($nRecordOnly.Keeper -eq '' -and $nRecordOnly.WorldName -eq '') `
    ("keeper='$($nRecordOnly.Keeper)' worldName='$($nRecordOnly.WorldName)'")

# The launcher's profile is the LIVE file - what this installation is running now
# and where `profile set` writes - so it wins over the record behind it.
@{ format = 'bibites-multiverse/launcher-profile/1'; name = 'default'; dataRoot = $nData
   keeper = 'Nightjar'; worldName = 'The Deep' } |
    ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $nProfilePath -Encoding ASCII
$nBoth = Get-PreviousPublicNames -DataRoot $nData -ProgramRoot $nProgram
Check "the launcher's profile wins over the install record" `
    ($nBoth.Keeper -eq 'Nightjar' -and $nBoth.WorldName -eq 'The Deep') `
    ("keeper='$($nBoth.Keeper)' worldName='$($nBoth.WorldName)'")

# A profile that describes ANOTHER world's folder says nothing about this one,
# which is the same rule the installer's Get-PreviousInstall applies.
@{ format = 'bibites-multiverse/launcher-profile/1'; name = 'default'
   dataRoot = (Join-Path $nRoot 'somewhere-else'); keeper = 'Mallory'; worldName = 'Not yours' } |
    ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $nProfilePath -Encoding ASCII
$nElsewhere = Get-PreviousPublicNames -DataRoot $nData -ProgramRoot $nProgram
Check "a profile for another data root is not read" `
    ($nElsewhere.Keeper -eq '' -and $nElsewhere.WorldName -eq '') `
    ("keeper='$($nElsewhere.Keeper)' worldName='$($nElsewhere.WorldName)'")
Remove-Item -LiteralPath $nProfilePath -Force
Remove-Item -LiteralPath $nRecordPath -Force

# WHAT THE BOXES REFUSE, on the form, BEFORE the child process starts. The
# installer refuses an over-long name at step 8 - after enrollment and after the
# certificate - and in a hidden console that refusal is a line in a log nobody is
# reading, over a half-changed machine. The window applies the same two bounds.
Check "a name inside the bound is accepted" ((Get-PublicNameProblem ('a' * 64)) -eq '')
Check "a name one byte over it is refused" ((Get-PublicNameProblem ('a' * 65)) -ne '')
Check "the bound is in BYTES, not characters" `
    ((Get-PublicNameProblem ([string]([char]0x00E9) * 33)) -ne '')
Check "a control character is refused" ((Get-PublicNameProblem "a`tb") -ne '')
Check "an empty box is the decline and is passed as '-'" `
    ((Get-PublishedNameArgument '  ') -eq '-')
Check "and a typed name is passed trimmed" `
    ((Get-PublishedNameArgument '  Alice  ') -eq 'Alice')

# HOW THE WINDOW HANDS ITS ANSWERS OVER. There is no argument array underneath a
# Windows process: it is given one string and splits it itself. A value ending in
# a backslash, wrapped in quotes the obvious way, escapes its own closing quote
# and swallows the flag behind it.
Check "an ordinary value is quoted" ((ConvertTo-NativeArgument 'Alice') -eq '"Alice"')
Check "a value with a space stays one argument" `
    ((ConvertTo-NativeArgument "Alice's world") -eq '"Alice''s world"')
Check "a trailing backslash is doubled before the closing quote" `
    ((ConvertTo-NativeArgument 'C:\Games\The Bibites\') -eq '"C:\Games\The Bibites\\"')
Check "an embedded quote is escaped" `
    ((ConvertTo-NativeArgument 'Bob "the Builder"') -eq '"Bob \"the Builder\""')
Check "and a backslash run before one is doubled first" `
    ((ConvertTo-NativeArgument 'a\\"b') -eq '"a\\\\\"b"')

# AND THE SAME STRING, THROUGH WINDOWS' OWN SPLITTER. CommandLineToArgvW is what
# every process on this machine takes its arguments apart with, so it is the only
# authority worth checking the quoting against. If it cannot be called here - a
# host with no compiler for Add-Type - the literal checks above still stand and
# this says so rather than passing quietly.
$nSplitter = $null
try {
    if (-not ('BibitesNativeCommandLine' -as [type])) {
        Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public static class BibitesNativeCommandLine {
    [DllImport("shell32.dll", SetLastError = true, CharSet = CharSet.Unicode)]
    private static extern IntPtr CommandLineToArgvW(string lpCmdLine, out int pNumArgs);
    [DllImport("kernel32.dll")]
    private static extern IntPtr LocalFree(IntPtr hMem);
    public static string[] Split(string commandLine) {
        int count;
        IntPtr block = CommandLineToArgvW(commandLine, out count);
        if (block == IntPtr.Zero) { throw new InvalidOperationException("CommandLineToArgvW failed"); }
        try {
            string[] args = new string[count];
            for (int i = 0; i < count; i++) {
                args[i] = Marshal.PtrToStringUni(Marshal.ReadIntPtr(block, i * IntPtr.Size));
            }
            return args;
        } finally { LocalFree(block); }
    }
}
'@
    }
    $nSplitter = [BibitesNativeCommandLine]
} catch {
    Write-Host ("  SKIP  Windows' own argument splitter is not available here: {0}" -f $_.Exception.Message) `
        -ForegroundColor Yellow
}
if ($nSplitter) {
    # The values a person can actually put in those boxes, and the two paths the
    # window passes beside them. argv[0] is parsed by different rules, so a
    # stand-in program name goes first and is dropped.
    $nValues = @('Alice', "Alice's world", 'C:\Games\The Bibites\', 'Bob "the Builder"',
                 '-', 'a b  c', 'Tom & Jerry')
    $nLine = (@('"prog"') + @($nValues | ForEach-Object { ConvertTo-NativeArgument $_ })) -join ' '
    $nBack = @($nSplitter::Split($nLine))
    Check "Windows splits the window's command line back into the same arguments" `
        ($nBack.Count -eq ($nValues.Count + 1) -and
         (0..($nValues.Count - 1) | ForEach-Object { $nBack[$_ + 1] -ceq $nValues[$_] }) -notcontains $false) `
        ("count=$($nBack.Count) back=" + (($nBack | Select-Object -Skip 1) -join ' | '))
}

# ------------------------------- and the installer's own answer to "was this asked?"
#
# WHAT MAKES A QUESTION ANSWERED, in the installer's own words, lifted out of the
# file that ships and run here. This is the pair that has to agree: the ADOPTION
# takes a value from the launcher's profile or, failing that, the install record,
# and the ANSWERED-CHECK has to consult exactly the same two in the same order.
# It did not - it read the profile alone - so a data root whose record held a
# decline and whose profile was gone counted as never asked, and an installer
# with somebody at the keyboard offered the Windows account name over it.
$installerAst = [System.Management.Automation.Language.Parser]::ParseFile($installer, [ref]$null, [ref]$null)
$previousSettingSource = ''
foreach ($fname in @('Find-PreviousSetting', 'Get-PreviousSetting', 'Test-PreviousSetting')) {
    $fnAst = $installerAst.Find({
        param($node)
        $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
        $node.Name -eq $fname }, $true)
    Check "the installer defines $fname" ($null -ne $fnAst)
    if ($fnAst) { $previousSettingSource += $fnAst.Extent.Text + "`n" }
}
. ([scriptblock]::Create($previousSettingSource))

function New-PreviousInstall {
    # $ProfileData, not $Profile: $profile is one of PowerShell's own automatic
    # variables, and the installer this stands in for says the same thing where
    # it reads the file.
    param($ProfileData, $RecordData)
    return [pscustomobject]@{
        Present = $true; Record = $RecordData; Profile = $ProfileData; ProfilePath = ''; Active = '' }
}
$pFromProfile = New-PreviousInstall ([pscustomobject]@{ keeper = 'Nightjar' }) $null
$pFromRecord  = New-PreviousInstall $null `
    ([pscustomobject]@{ settings = [pscustomobject]@{ keeper = '' } })
$pFromNeither = New-PreviousInstall $null ([pscustomobject]@{ world = 'Eden' })
$pFresh = [pscustomobject]@{ Present = $false; Record = $null; Profile = $null; ProfilePath = ''; Active = '' }

Check "a name in the launcher's profile counts as answered" `
    (Test-PreviousSetting $pFromProfile 'keeper' 'settings.keeper')
Check "a DECLINE in the install record alone counts as answered too" `
    (Test-PreviousSetting $pFromRecord 'keeper' 'settings.keeper')
Check "and it is adopted as the empty value it is, not as a default" `
    ((Get-PreviousSetting $pFromRecord $false 'keeper' 'settings.keeper' 'fallback') -eq '')
Check "a record that carries no such key has never been asked" `
    (-not (Test-PreviousSetting $pFromNeither 'keeper' 'settings.keeper'))
Check "and there the caller keeps its own default" `
    ((Get-PreviousSetting $pFromNeither $false 'keeper' 'settings.keeper' 'fallback') -eq 'fallback')
Check "a computer with no install here has been asked nothing" `
    (-not (Test-PreviousSetting $pFresh 'keeper' 'settings.keeper'))
Check "and a flag named on this run is never overridden by history" `
    ((Get-PreviousSetting $pFromProfile $true 'keeper' 'settings.keeper' 'named') -eq 'named')

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
Check "it says what an install that got past step 0 had already done" `
    ($busyRefusal -match 'WHAT THIS INSTALL HAD ALREADY DONE')
# THE LATE REFUSAL IS REACHED FROM TWO STEPS, so it must not name either: the mod
# copy at step 5 and the program copy at step 9 both go through Copy-ProgramFile,
# and a sentence that claimed the identity was settled would be wrong at step 5.
Check "and it does not claim which steps those were" `
    (-not ($busyRefusal -match 'WHAT THIS INSTALL HAD ALREADY DONE: BepInEx'))
Check "the mod is copied through the same lock-aware copy the launcher is" `
    ($installerCode -match 'Copy-ProgramFile \$pluginSrc \$pluginDst')
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

# THE MANAGED GAME COPY, AND THE CYCLE THAT COULD NOT RUN. Install, install
# again over the same managed runtime, uninstall, install again: the second
# install found BepInEx already in <data root>\runtimes\<sha>, called it somebody
# else's, and recorded installedByThisInstaller=false with an empty file list.
# The uninstall then left the whole framework where it was while removing the
# game payload it HAD recorded, and the install after that found a folder with
# BepInEx in it and no game, refused to overwrite it, and named a file
# (changelog.txt) that nothing could put back. These read CODE LINES ONLY, so
# the comments that explain the rule cannot satisfy the check.
$uninstallerCode = ((Get-Content -LiteralPath $uninstaller) | Where-Object { $_ -notmatch '^\s*#' }) -join "`n"
Check "one rule decides whose BepInEx a game folder holds" `
    ($installerCode -match 'function Test-InstallerOwnedBepInEx')
Check "step 4 asks that rule instead of asking only whether BepInEx is there" `
    ($installerCode -match 'Test-InstallerOwnedBepInEx -Present \$bepInExHeld')
Check "BepInEx already in the managed runtime is this install's" `
    ($installerCode -match '(?s)function Test-InstallerOwnedBepInEx.*?if \(\$RuntimeMode -eq ''bundled'' -and \(Test-ManagedRuntimePath \$GameDir \$DataRoot\)\) \{ return \$true \}')
Check "BepInEx already in a game folder somebody else chose is not" `
    ($installerCode -match '(?s)function Test-InstallerOwnedBepInEx.*?if \(-not \$Present\) \{ return \$true \}')
# THE UPGRADE HALF OF THE SAME RULE. A framework this installer unpacked into
# somebody's own game folder on install one is still this install's on install
# two, and the previous record is the only evidence there is for that: without it
# the second install writes the empty file list again and the uninstall leaves
# the whole framework in a folder it promised to put back as it found it.
Check "BepInEx a previous install of this package put there is still this install's" `
    ($installerCode -match '(?s)function Test-InstallerOwnedBepInEx.*?return \$PreviouslyOurs')
Check "step 4 asks the previous record whose framework it is" `
    ($installerCode -match '-PreviouslyOurs \$bepInExWasOurs')
Check "and it only believes a record that names this very game folder" `
    ($installerCode -match 'Test-SamePath \(Get-JsonField \$previousInstall\.Record ''gameDir''\) \$GameDir')
Check "a file of the framework already on disk is recorded when the game copy is this package's" `
    ($installerCode -match '\$bepInExManaged -or \$bepInExPreviouslyRecorded\.ContainsKey\(\$relKey\)')
Check "and a file the previous install recorded is recorded again on an upgrade" `
    ($installerCode -match '\$bepInExPreviouslyRecorded\[\$previousRel\.ToUpperInvariant\(\)\] = \$true')
Check "and the game's own files are never recorded as the framework's" `
    ($installerCode -match '\$payloadOwns\[\$file\.relative\.ToUpperInvariant\(\)\] = \$true')
Check "nothing is unpacked over a framework that is already in the managed game copy" `
    ($installerCode -match '(?s)if \(\$bepInExHeld\) \{.*?\} else \{\s*\n\s*Expand-Archive')

# Step 2 rebuilds a managed runtime that is no longer a game, rather than
# refusing an install nothing could get past. The refusal it replaced is gone,
# and the one for a runtime that is COMPLETE and DIFFERENT is not.
Check "step 2 no longer refuses an incomplete managed runtime" `
    (-not ($installerCode -match 'is incomplete \(\$\(\$file\.relative\) is missing\)'))
Check "step 2 still refuses a managed runtime that is complete and changed" `
    ($installerCode -match 'was changed \(\$\(\$changedFiles\[0\]\) differs\)')
Check "step 2 stages the payload again after removing an incomplete runtime" `
    ($installerCode -match '\$stageRuntime = \$true' -and $installerCode -match 'if \(\$stageRuntime\) \{')
Check "step 2 counts what it is about to remove, on screen" `
    ($installerCode -match '\$leftBehind = @\(Get-ChildItem -LiteralPath \$runtimeRoot -File -Force -Recurse')

# NOTHING IS DELETED AT A PATH EITHER SCRIPT CANNOT PROVE IS THE MANAGED GAME
# COPY. Both carry the same rule, and both ask it before a recursive delete.
foreach ($pair in @(@{ name = 'installer'; text = $installerCode },
                    @{ name = 'uninstall'; text = $uninstallerCode })) {
    Check ("the $($pair.name) has the managed-runtime path rule") `
        ($pair.text -match 'function Test-ManagedRuntimePath')
    Check ("the $($pair.name) rule refuses a path with .. in it") `
        ($pair.text -match '(?s)function Test-ManagedRuntimePath.*?if \(\$segment -eq ''\.\.''\) \{ return \$false \}')
    Check ("the $($pair.name) rule requires the parent to be the runtimes folder itself") `
        ($pair.text -match '(?s)function Test-ManagedRuntimePath.*?\$parent\.TrimEnd\(''\\'', ''/''\) -ne \$runtimes\.TrimEnd')
    Check ("the $($pair.name) rule requires the leaf to be a sha256") `
        ($pair.text -match '(?s)function Test-ManagedRuntimePath.*?-notmatch ''\^\[0-9A-Fa-f\]\{64\}\$''')
}
Check "step 2 proves the path against the payload's own sha before removing it" `
    ($installerCode -match 'if \(-not \(Test-ManagedRuntimePath \$runtimeRoot \$DataRoot \$payloadSha\)\)')
Check "and refuses rather than deleting when it cannot" `
    ($installerCode -match '(?s)Test-ManagedRuntimePath \$runtimeRoot \$DataRoot \$payloadSha\)\) \{\s*\n\s*Stop-Setup')

# The uninstall's own half: a managed runtime nothing of this install's is left
# in is rubble, and rubble is what the next install cannot repair.
Check "the uninstall asks whether any file it recorded survived" `
    ($uninstallerCode -match '\$runtimeSurvivors \+= \[string\]\$file\.path')
Check "it asks the hashes rather than the ledger, so -DryRun answers honestly" `
    ($uninstallerCode -match '(?s)\$runtimeSurvivors = @\(\).*?if \(\(Get-Sha256 \$file\.path\) -eq \(\[string\]\$file\.sha256\)\.ToUpperInvariant\(\)\) \{ continue \}')
Check "it counts the residue the game and BepInEx left, and nothing it recorded" `
    ($uninstallerCode -match '\$recordedRuntimePaths\.ContainsKey\(\$_\.FullName\.ToUpperInvariant\(\)\)')
Check "it sweeps the managed game copy only when nothing of this install's is left" `
    ($uninstallerCode -match 'if \(\$runtimeSurvivors\.Count -eq 0 -and \$runtimeResidue\.Count -gt 0 -and')
Check "and only at a path the managed-runtime rule accepts" `
    ($uninstallerCode -match '\(Test-ManagedRuntimePath \$runtimeRoot \$recordedDataRoot\)\)')
Check "a surviving changed file keeps its folder, one directory at a time" `
    ($uninstallerCode -match '(?s)\} else \{\s*\n\s*\$runtimeDirs = @\(Get-ChildItem -LiteralPath \$runtimeRoot -Directory')
Check "the sweep says how many files it removed beyond its own record" `
    ($uninstallerCode -match 'file\(s\) created by the game and BepInEx after the install')

# THE APPLICATION FOLDER HAS TO END UP EMPTY, or the setup's own uninstaller
# cannot take its directory away. Test-SafeDataRoot refuses a data root that
# CONTAINS a protected path, and the game folder is protected - but the complete
# edition's game folder is INSIDE the data root, so every complete-edition data
# root failed the check, every profile was kept "because its data root is not a
# path this script will act on", profiles\ stayed, and the folder holding it
# stayed with it.
Check "the protected paths go through one filter" `
    ($uninstallerCode -match 'function Get-ProtectedRoot')
Check "it drops only the game copy this run reclaims itself" `
    ($uninstallerCode -match '(?s)function Get-ProtectedRoot.*?if \(Test-ManagedRuntimePath \(\[string\]\$path\) \$DataRoot\) \{ continue \}')
Check "the install's own game folder goes through it" `
    ($uninstallerCode -match '\$protectedRoots = @\(Get-ProtectedRoot @\(\[string\]\$record\.gameDir')
Check "and so does every profile's own game folder, at every place that asks" `
    ((([regex]::Matches($uninstallerCode, 'Get-ProtectedRoot @\(\[string\]\$\w+Data\.gameDir\)')).Count) -ge 3)
Check "the data root the filter measures against is the record's own" `
    ($uninstallerCode -match '\$recordedDataRoot = \[string\]\$record\.dataRoot')
Check "containment is compared with the platform's own separator" `
    ($uninstallerCode -match '\$me \+ \[System\.IO\.Path\]::DirectorySeparatorChar')

# "Read the uninstall ledger" is a remedy in docs/error-taxonomy.md, and until
# this file existed it named nothing: the ledger went to a console window that
# the entry in Installed apps closes the moment the script ends.
Check "every line the uninstall prints goes through one function" `
    ($uninstallerCode -match 'function Write-Screen')
Check "its own Say and Step go through it too" `
    ($uninstallerCode -match 'function Say  \{ param\(\[string\]\$m\) Write-Screen')
Check "the uninstall writes its ledger to a file" `
    ($uninstallerCode -match 'function Save-Ledger')
Check "beside the world's own logs, named like the install log" `
    ($uninstallerCode -match "'uninstall-' \+ \(Get-Date\)\.ToUniversalTime\(\)\.ToString\('yyyyMMddTHHmmssZ'\)")
Check "in %TEMP% when the run deletes the folder that would hold it" `
    ($uninstallerCode -match 'if \(\$RemoveWorldData -and \$env:TEMP\) \{ \$ledgerDir = \$env:TEMP \}')
Check "a refusal keeps its ledger too" `
    ($uninstallerCode -match '(?s)function Stop-Uninstall \{.*?Save-Ledger.*?exit 1')
Check "a dry run writes nothing, this file included" `
    ($uninstallerCode -match '(?s)function Save-Ledger \{.*?if \(\$DryRun\) \{')
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
    # THE INSTALLER ASKS FOR THE TWO NAMES A WORLD IS PUBLISHED UNDER when there
    # is somebody at the keyboard, and this suite runs it from a console that has
    # one - so every call says -Unattended, and a scenario cannot sit for ever at
    # a question nobody is watching.
    #
    # -Unattended IS NOT AN ANSWER. It says "ask nothing", so a run that names no
    # -Keeper publishes none and an upgrade still keeps what the installation had
    # - which is exactly what scenario H measures. A scenario that means to set
    # one passes it.
    if ((Split-Path -Leaf $Path) -eq 'Install-BibitesMultiverse.ps1' -and
        -not $Arguments.ContainsKey('Unattended')) {
        $Arguments = $Arguments.Clone()
        $Arguments['Unattended'] = $true
    }
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
                       'saveOnQuit', 'peerId', 'relayUrl', 'createdUtc', 'keeper', 'worldName')) {
        Check ("the profile carries $key") ($profileKeys -contains $key) ($profileKeys -join ', ')
    }
    Check "the profile carries nothing else" ($profileKeys.Count -eq 17) ($profileKeys -join ', ')
    # AN INSTALL NOBODY WAS ASKED ANYTHING BY PUBLISHES NO NAME. The two public
    # strings are written as empty rather than left out - the launcher's own
    # writer emits the same seventeen keys, and a test in go/internal/launcher
    # parses this very file to keep the two writers together - and neither of
    # them is the account name of whoever ran this.
    Check "an unattended install published no keeper and no world name" `
        ($profileObj.keeper -eq '' -and $profileObj.worldName -eq '') `
        ("keeper='$($profileObj.keeper)' worldName='$($profileObj.worldName)'")
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
# The sidecar's diagnostic looks the game build up in this file, beside its own
# executable. Without it here, 'Run a health check' answers UNKNOWN for the
# game-version check on an install that is perfectly healthy.
Check "the application directory contains the support matrix the diagnostic reads" `
    (Test-Path -LiteralPath (Join-Path $fProgram 'support-matrix.json'))
Check "the application directory contains the launcher's default profile" `
    (Test-Path -LiteralPath (Join-Path $fProgram 'profiles\default.json'))
$startF = Get-Content -Raw -LiteralPath (Join-Path $fProgram 'Start-Multiverse.ps1')
Check "the generated start script points at the managed runtime" ($startF.Contains($fRuntime))

# THE CYCLE THAT COULD NOT RUN, end to end. Install, run a world, install again
# over the same managed runtime, uninstall, install again. The release before
# this fix stopped dead at the last step - "The managed runtime at ... is
# incomplete (changelog.txt is missing). It was not overwritten." - with no way
# past it but deleting a folder by hand.

# What the game and BepInEx write into the game folder once a world has run.
$fBepInEx = Join-Path $fRuntime 'BepInEx'
New-Item -ItemType Directory -Force -Path (Join-Path $fBepInEx 'cache') | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $fBepInEx 'config') | Out-Null
Set-Content -LiteralPath (Join-Path $fBepInEx 'LogOutput.log') -Value 'a world ran here' -Encoding ASCII
Set-Content -LiteralPath (Join-Path $fBepInEx 'config\BepInEx.cfg') -Value '[Logging]' -Encoding ASCII
Set-Content -LiteralPath (Join-Path $fBepInEx 'cache\chainloader_typeloader.dat') -Value 'cache' -Encoding ASCII

$install = Invoke-Script $fInstaller @{
    DataRoot = $fData; InstallRoot = $fProgram; JoinStringFile = $fJoin
}
Check "installing again over the managed runtime succeeded" ($install.ExitCode -eq 0) $install.Output
Check "it reused the game copy it had already verified" `
    ($install.Output -match 'reusing the verified managed game runtime')
$recordF2 = Get-Content -Raw -LiteralPath (Join-Path $fData 'install-record.json') | ConvertFrom-Json
# THE BUG THAT MADE THE HUSK. This install found BepInEx already in the managed
# runtime - because the install before it put it there - and recorded it as
# somebody else's, with no files. The uninstall then left the whole framework
# behind while removing the game payload around it.
Check "it says the framework in that folder is its own" `
    ($install.Output -match "already in this package's own managed game copy")
Check "the second install still owns the BepInEx in its own managed runtime" `
    ($recordF2.bepInEx.installedByThisInstaller -eq $true)
Check "and records its files, so the uninstall can take them" `
    (@($recordF2.bepInEx.files).Count -gt 0)

Set-Content -LiteralPath (Join-Path $fRuntime 'user-note.txt') -Value 'left in the game copy' -Encoding ASCII
$fUninstaller = Join-Path $fProgram 'Uninstall-BibitesMultiverse.ps1'
$uninstall = Invoke-Script $fUninstaller @{ DataRoot = $fData }
Check "the complete uninstall succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
Check "the unchanged game executable was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fRuntime 'The Bibites.exe')))
# Nothing this install recorded is left in that folder, so the folder is not a
# game any more - and this package's own copy of the game, with the log, the
# config and the cache the game wrote inside it, goes whole rather than staying
# behind as something the next install cannot repair.
Check "the managed game copy went whole, residue and all" `
    (-not (Test-Path -LiteralPath $fRuntime))
Check "the uninstall said how much it removed beyond its own record" `
    ($uninstall.Output -match 'file\(s\) created by the game and BepInEx after the install')
Check "the runtimes folder went with the last runtime in it" `
    (-not (Test-Path -LiteralPath (Join-Path $fData 'runtimes')))
# "Read the uninstall ledger" is a remedy in docs/error-taxonomy.md, and it has
# to name something a reader can still open after the window has closed.
$fLedgers = @(Get-ChildItem -LiteralPath (Join-Path $fData 'logs') -Filter 'uninstall-*.log' `
                            -File -ErrorAction SilentlyContinue)
Check "the uninstall kept a ledger of its own" ($fLedgers.Count -ge 1)
if ($fLedgers.Count -ge 1) {
    $fLedgerText = Get-Content -Raw -LiteralPath ($fLedgers | Sort-Object Name | Select-Object -Last 1).FullName
    Check "that ledger holds what was removed and what was kept" `
        ($fLedgerText -match 'what was removed' -and $fLedgerText -match 'what was kept')
}
Check "the installed sidecar was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'multiverse-sidecar.exe')))
Check "the installed launcher was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'BibitesMultiverseLauncher.exe')))
Check "the installed launcher's commands were removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'multiverse-launcher.exe')))
Check "the installed public map was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'public-map.json')))
Check "the installed support matrix was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'support-matrix.json')))
Check "the launcher's profiles directory was removed" `
    (-not (Test-Path -LiteralPath (Join-Path $fProgram 'profiles')))
# A profile describes an installation that no longer exists, so it goes with it -
# and the complete edition's own profile used to be kept, because its data root
# holds the managed game copy and the check that decides what may be acted on
# protected that copy from the run removing it.
Check "the profile was removed rather than kept for its data root" `
    (-not ($uninstall.Output -match 'not a path this script will act on'))
Check "the application directory itself went, so the setup's own uninstaller can take it" `
    (-not (Test-Path -LiteralPath $fProgram))

# THE INSTALL THAT USED TO DIE. Nothing about this run is special: the same
# package, the same data root, the same world.
$install = Invoke-Script $fInstaller @{
    DataRoot = $fData; InstallRoot = $fProgram; JoinStringFile = $fJoin
}
Check "installing again after the uninstall succeeded" ($install.ExitCode -eq 0) $install.Output
Check "nothing about the managed runtime was refused" (-not ($install.Output -match 'INS-RUNTIME'))
Check "it staged the game payload again" `
    ($install.Output -match 'installed the verified game payload into a managed runtime')
Check "the game is back in the managed runtime" `
    (Test-Path -LiteralPath (Join-Path $fRuntime 'The Bibites.exe'))

# THE HUSK THE UNINSTALL BEFORE THIS FIX LEFT BEHIND, made here exactly as that
# release made it: every recorded game file gone, the framework still there. A
# machine that already carries one is healed by an install rather than refused.
$recordF3 = Get-Content -Raw -LiteralPath (Join-Path $fData 'install-record.json') | ConvertFrom-Json
foreach ($payloadFile in @($recordF3.runtime.files)) {
    Remove-Item -LiteralPath ([string]$payloadFile.path) -Force -ErrorAction SilentlyContinue
}
Check "the husk has no game left in it" `
    (-not (Test-Path -LiteralPath (Join-Path $fRuntime 'The Bibites.exe')))
Check "and still holds the mod framework" (Test-Path -LiteralPath (Join-Path $fRuntime 'BepInEx'))
$install = Invoke-Script $fInstaller @{
    DataRoot = $fData; InstallRoot = $fProgram; JoinStringFile = $fJoin
}
Check "an install over that husk succeeded" ($install.ExitCode -eq 0) $install.Output
Check "it said what it found there" ($install.Output -match 'is not the game any more')
Check "it removed the incomplete copy whole" `
    ($install.Output -match 'removed the incomplete managed runtime whole')
Check "and put the game back" (Test-Path -LiteralPath (Join-Path $fRuntime 'The Bibites.exe'))

# A GAME FILE SOMEBODY CHANGED IS STILL KEPT, and keeps the folder around it.
# The sweep is for rubble; a copy something in it is still vouching for is not
# rubble, and neither script decides that for you.
$recordF4 = Get-Content -Raw -LiteralPath (Join-Path $fData 'install-record.json') | ConvertFrom-Json
$fChangedPayload = [string](@($recordF4.runtime.files)[0].path)
Add-Content -LiteralPath $fChangedPayload -Value 'changed by hand'
$uninstall = Invoke-Script (Join-Path $fProgram 'Uninstall-BibitesMultiverse.ps1') @{ DataRoot = $fData }
Check "the uninstall after a hand-changed game file succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
Check "it kept that file and said why" ($uninstall.Output -match 'CHANGED since the install')
Check "the changed file is still there" (Test-Path -LiteralPath $fChangedPayload)
Check "and the folder around it stayed" (Test-Path -LiteralPath $fRuntime)

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

# ---------------------------------------------------------------- H

# THE UPGRADE, WHICH IS THE ONLY THING THE HOMEPAGE DOWNLOAD EVER DOES ON A
# MACHINE THAT ALREADY HAS THIS. The setup executable passes no settings at all,
# so before this scenario existed a participant who had renamed their world,
# moved its port, or told it to run without a window got every one of those
# reset by a run whose whole purpose was to hand them a newer build - and the
# launcher then opened a save that was not the one they had been playing.
#
# The shape is install, CHANGE THINGS THE WAY A PARTICIPANT WOULD, upgrade with
# no flags at all, and then check that the changes are still there and that the
# journal, the logs and the credential were never touched. It ends in an
# uninstall, because what an upgrade records is what the uninstall can put back.
Scenario "H - an upgrade over an existing install"

$hRoot    = Join-Path $sandbox 'H'
$hGame    = Join-Path $hRoot 'game'
$hData    = Join-Path $hRoot 'data'
$hProgram = Join-Path $hRoot 'program'
New-SandboxGame -Path $hGame
$hJoin   = Join-Path $hRoot 'join.txt'
$hSecret = New-JoinFile $hJoin

$install = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $hGame; DataRoot = $hData
    InstallRoot = $hProgram; JoinStringFile = $hJoin
    World = 'FirstWorld'; SidecarPort = 8891; SaveMinutes = 4; SaveKeep = 3; ExportEdges = 'E,N'
    # The two names this world is published under, answered on the command line
    # the way the graphical setup answers them. The world name carries an
    # apostrophe on purpose: it is what the installer itself offers, and it is
    # the character that would end a quoted string in the script it generates.
    Keeper = 'Alice'; WorldName = "Alice's world"
}
Check "the first install succeeded" ($install.ExitCode -eq 0) $install.Output
$hProfilePath = Join-Path $hProgram 'profiles\default.json'
$hActivePath  = Join-Path $hProgram 'profiles\active.txt'
$hRecordPath  = Join-Path $hData 'install-record.json'
$hCredential  = Join-Path $hData 'peer-secret.txt'
Check "it did not say it was updating anything" `
    (-not ($install.Output -match 'updating the Bibites Multiverse')) $install.Output

# WHAT A PARTICIPANT DOES NEXT, done here the way the launcher does it: the
# profile is edited, a second world is added beside it, and the installation is
# left opening on that second world.
$hProfile = Get-Content -Raw -LiteralPath $hProfilePath | ConvertFrom-Json
$hProfile.world       = 'Eden'
$hProfile.sidecarPort = 8899
$hProfile.headless    = $true
$hProfile.saveKeep    = 9
$hCreatedUtc          = $hProfile.createdUtc
$hProfile | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $hProfilePath -Encoding ASCII
$hSecondProfile = Join-Path $hProgram 'profiles\second.json'
Copy-Item -LiteralPath $hProfilePath -Destination $hSecondProfile -Force
Set-Content -LiteralPath $hActivePath -Value 'second' -Encoding ASCII

# The world's own files: the journal it is holding for other worlds, and the
# logs beside it. Neither is ever an installer's to touch.
$hJournal = Join-Path $hData 'data\journal.ndjson'
Set-Content -LiteralPath $hJournal -Value 'one line of a world''s custody' -Encoding ASCII
$hLog = Join-Path $hData 'logs\sidecar.log'
Set-Content -LiteralPath $hLog -Value 'a log line' -Encoding ASCII
$hDataBefore = Get-TreeSnapshot (Join-Path $hData 'data')
$hLogBefore  = (Get-FileHash -Path $hLog -Algorithm SHA256).Hash

# A FILE THE RELEASE BEFORE THIS ONE SHIPPED AND THIS ONE DOES NOT, recorded the
# way the previous install would have recorded it, plus one of the same kind that
# somebody has since edited. The first is this setup's to remove; the second is
# not, and it says so.
$hStale  = Join-Path $hProgram 'retired-tool.exe'
$hTouched = Join-Path $hProgram 'retired-and-edited.exe'
Set-Content -LiteralPath $hStale   -Value 'a program an older release shipped' -Encoding ASCII
Set-Content -LiteralPath $hTouched -Value 'another one' -Encoding ASCII
$hRecord = Get-Content -Raw -LiteralPath $hRecordPath | ConvertFrom-Json
$hRecord.program.files = @($hRecord.program.files) + @(
    [pscustomobject]@{ path = $hStale;   sha256 = (Get-FileHash -Path $hStale   -Algorithm SHA256).Hash },
    [pscustomobject]@{ path = $hTouched; sha256 = (Get-FileHash -Path $hTouched -Algorithm SHA256).Hash })
$hRecord | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $hRecordPath -Encoding ASCII
Add-Content -LiteralPath $hTouched -Value 'and a line somebody added' -Encoding ASCII

# THE UPGRADE: exactly the arguments the setup executable passes, and not one
# setting among them.
$upgrade = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $hGame; DataRoot = $hData; InstallRoot = $hProgram
}
Check "the upgrade succeeded" ($upgrade.ExitCode -eq 0) $upgrade.Output
Check "it said it was updating what was already here" `
    ($upgrade.Output -match 'updating the Bibites Multiverse') $upgrade.Output
Check "it said the settings were kept rather than reset" `
    ($upgrade.Output -match 'keeping the settings this installation already had') $upgrade.Output
Check "it reused the identity already in the data root" `
    ($upgrade.Output -match 'reusing the map identity already in') $upgrade.Output
Check "it asked the map for nothing" `
    (-not ($upgrade.Output -match 'requesting a unique identity')) $upgrade.Output

$hAfter = Get-Content -Raw -LiteralPath $hProfilePath | ConvertFrom-Json
Check "the upgrade kept the world the launcher was opening" ($hAfter.world -eq 'Eden')
Check "and the port it had been moved to" ($hAfter.sidecarPort -eq 8899)
Check "and the window setting only the launcher can set" ($hAfter.headless -eq $true)
Check "and how many saves it was keeping" ($hAfter.saveKeep -eq 9)
Check "and the edges the first install was given" ($hAfter.exportEdges -eq 'E,N')
Check "an upgrade is not a creation, so the world is not made younger" `
    ($hAfter.createdUtc -eq $hCreatedUtc)
# THE TWO PUBLISHED NAMES SURVIVE AN UPGRADE, and an upgrade that names neither
# does not ask again either: a participant is asked once, and a run that says
# -Unattended and nothing else keeps what this installation already published.
Check "the upgrade kept the names this world is published under" `
    ($hAfter.keeper -eq 'Alice' -and $hAfter.worldName -eq "Alice's world") `
    ("keeper='$($hAfter.keeper)' worldName='$($hAfter.worldName)'")
Check "the world's identity is untouched" ($hAfter.peerId -eq 'test-world')
Check "the second world beside it is still there" (Test-Path -LiteralPath $hSecondProfile)
Check "and the installation still opens on it" `
    ((Get-Content -Raw -LiteralPath $hActivePath).Trim() -eq 'second')

$hRecordAfter = Get-Content -Raw -LiteralPath $hRecordPath | ConvertFrom-Json
Check "the record carries the kept settings too" `
    ($hRecordAfter.world -eq 'Eden' -and [int]$hRecordAfter.sidecarPort -eq 8899 -and
     $hRecordAfter.settings.exportEdges -eq 'E,N' -and [int]$hRecordAfter.settings.saveKeep -eq 9)
$hStart = Get-Content -Raw -LiteralPath (Join-Path $hProgram 'Start-Multiverse.ps1')
Check "and so does the start script, so the two describe one world" `
    ($hStart -match "'Eden'" -and $hStart -match '8899')
# THE APOSTROPHE IS THE POINT. A world name is carried into that script inside a
# single-quoted PowerShell string, where one apostrophe would end the string and
# turn the rest of the line into code; it is doubled, and the script has to parse
# and to hold the name that was typed.
Check "the start script carries the two published names" `
    ($hStart -match "\`$Keeper\s+= 'Alice'" -and $hStart -match "\`$WorldName\s+= 'Alice''s world'") $hStart
$hStartErrors = $null
$hStartAst = [System.Management.Automation.Language.Parser]::ParseInput($hStart, [ref]$null, [ref]$hStartErrors)
Check "and the start script still parses with a name that holds an apostrophe" `
    ($hStartErrors.Count -eq 0) (($hStartErrors | ForEach-Object { $_.Message }) -join '; ')

# AND IT HANDS THE NAME TO THE SIDECAR WHOLE. Start-Process joins -ArgumentList
# with spaces and quotes nothing, on 5.1 and on 7 alike, so an array reached the
# sidecar as "--world-name" "Alice's" "world" and a world published as
# "Alice's world" was on the map as "Alice's". The offered world name ALWAYS has
# a space in it, so this was every default install that answered the question.
Check "the start script builds one quoted command line for the sidecar" `
    ($hStart -match '\$sidecarCommandLine\s*=' -and
     $hStart -match '-ArgumentList \$sidecarCommandLine') $hStart
Check "and it does not hand Start-Process the bare array any more" `
    (-not ($hStart -match '-ArgumentList \$sidecarArgs')) $hStart
# The generated script's OWN quoting function, lifted out of the file this
# install just wrote and run here: what ships is what is tested.
$hQuoter = $hStartAst.Find({
    param($node)
    $node -is [System.Management.Automation.Language.FunctionDefinitionAst] -and
    $node.Name -eq 'ConvertTo-NativeArgument' }, $true)
Check "the start script defines the quoting it uses" ($null -ne $hQuoter) $hStart
if ($hQuoter) {
    # Into a scope of its own, so this cannot be confused with the window's copy
    # of the same function that this suite dot-sourced at the top.
    $hQuoted = & ([scriptblock]::Create($hQuoter.Extent.Text + @'

@("Alice's world", 'C:\data\root\', 'Bob "the Builder"', 'Tom & Jerry') |
    ForEach-Object { ConvertTo-NativeArgument $_ }
'@))
    Check "the generated script quotes a name with a space" ($hQuoted[0] -ceq '"Alice''s world"') $hQuoted[0]
    Check "and doubles a trailing backslash" ($hQuoted[1] -ceq '"C:\data\root\\"') $hQuoted[1]
    Check "and escapes an embedded quote" ($hQuoted[2] -ceq '"Bob \"the Builder\""') $hQuoted[2]
    Check "and leaves an ampersand alone, because Windows argument parsing has no shell in it" `
        ($hQuoted[3] -ceq '"Tom & Jerry"') $hQuoted[3]
}

Check "the journal is byte for byte what it was" `
    ((Compare-Snapshot $hDataBefore (Get-TreeSnapshot (Join-Path $hData 'data'))).Count -eq 0)
Check "and so is this world's own log" `
    ((Get-FileHash -Path $hLog -Algorithm SHA256).Hash -eq $hLogBefore)
Check "the credential was never rewritten" `
    ((Get-Content -Raw -LiteralPath $hCredential).Trim() -eq $hSecret)

Check "a file the release before this one shipped is gone" (-not (Test-Path -LiteralPath $hStale))
Check "the upgrade said it removed it" `
    ($upgrade.Output -match 'the release before this one shipped it') $upgrade.Output
Check "one somebody edited is KEPT instead" (Test-Path -LiteralPath $hTouched)
Check "and the upgrade said why" `
    ($upgrade.Output -match 'it has CHANGED since the') $upgrade.Output
Check "the launcher itself is still installed" `
    (Test-Path -LiteralPath (Join-Path $hProgram 'BibitesMultiverseLauncher.exe'))

# A SETTING NAMED ON THE COMMAND LINE IS AN INSTRUCTION, and history never wins
# over one. This is the other half of the rule and the one that would make the
# adoption a trap if it were missing.
$named = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $hGame; DataRoot = $hData; InstallRoot = $hProgram
    World = 'Named'; SidecarPort = 8877
}
Check "an install that names a setting succeeded" ($named.ExitCode -eq 0) $named.Output
$hNamed = Get-Content -Raw -LiteralPath $hProfilePath | ConvertFrom-Json
Check "the named world wins over the kept one" ($hNamed.world -eq 'Named')
Check "and so does the named port" ($hNamed.sidecarPort -eq 8877)
Check "everything it did not name is still kept" `
    ($hNamed.saveKeep -eq 9 -and $hNamed.exportEdges -eq 'E,N' -and $hNamed.headless -eq $true)

# ---- A DECLINE IS AN ANSWER, AND AN UPGRADE KEEPS IT ----
#
# The two published names are the only settings here whose EMPTY value is a
# choice somebody made. So the decline gets its own pass through the upgrade:
# taken deliberately with '-', kept by a run that names nothing, and kept when
# the launcher's profile is gone and the install record is all that is left -
# which is the source the installer's answered-check used to ignore.
$decline = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $hGame; DataRoot = $hData; InstallRoot = $hProgram
    Keeper = '-'; WorldName = '-'
}
Check "a run that declines both names succeeded" ($decline.ExitCode -eq 0) $decline.Output
Check "and said this world is shown as unknown" `
    ($decline.Output -match 'your world is shown as unknown') $decline.Output
$hDeclined = Get-Content -Raw -LiteralPath $hProfilePath | ConvertFrom-Json
Check "the decline is written as an empty value, not a missing one" `
    (($hDeclined.PSObject.Properties.Name -contains 'keeper') -and
     ($hDeclined.PSObject.Properties.Name -contains 'worldName') -and
     $hDeclined.keeper -eq '' -and $hDeclined.worldName -eq '') `
    ("keeper='$($hDeclined.keeper)' worldName='$($hDeclined.worldName)'")

$keptDecline = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $hGame; DataRoot = $hData; InstallRoot = $hProgram
}
Check "an upgrade after a decline succeeded" ($keptDecline.ExitCode -eq 0) $keptDecline.Output
$hStillDeclined = Get-Content -Raw -LiteralPath $hProfilePath | ConvertFrom-Json
Check "and the decline survived it" `
    ($hStillDeclined.keeper -eq '' -and $hStillDeclined.worldName -eq '') `
    ("keeper='$($hStillDeclined.keeper)' worldName='$($hStillDeclined.worldName)'")
Check "nobody's account name was published in its place" `
    ($hStillDeclined.keeper -ne $env:UserName)
$hDeclinedStart = Get-Content -Raw -LiteralPath (Join-Path $hProgram 'Start-Multiverse.ps1')
Check "and the start script sends the sidecar neither name" `
    ($hDeclinedStart -match "\`$Keeper\s+= ''" -and $hDeclinedStart -match "\`$WorldName\s+= ''") $hDeclinedStart

# THE INSTALL RECORD ON ITS OWN. A data root restored without its application
# folder, or a launcher profile somebody deleted, leaves the record as the only
# thing that remembers the answer - and it is an answer.
Remove-Item -LiteralPath $hProfilePath -Force
$hRecordDeclined = Get-Content -Raw -LiteralPath $hRecordPath | ConvertFrom-Json
Check "the record carries the decline for the profile to be rebuilt from" `
    (($hRecordDeclined.settings.PSObject.Properties.Name -contains 'keeper') -and
     $hRecordDeclined.settings.keeper -eq '' -and $hRecordDeclined.settings.worldName -eq '')
$fromRecord = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $hGame; DataRoot = $hData; InstallRoot = $hProgram
}
Check "an upgrade with no profile left succeeded" ($fromRecord.ExitCode -eq 0) $fromRecord.Output
$hRebuilt = Get-Content -Raw -LiteralPath $hProfilePath | ConvertFrom-Json
Check "and the record's decline is what the rebuilt profile carries" `
    ($hRebuilt.keeper -eq '' -and $hRebuilt.worldName -eq '') `
    ("keeper='$($hRebuilt.keeper)' worldName='$($hRebuilt.worldName)'")

# Put the names back, so what the uninstall at the end of this scenario takes
# apart is the world the rest of it described.
$restore = Invoke-Script $installer @{
    RuntimeSelection = 'external'; GameDir = $hGame; DataRoot = $hData; InstallRoot = $hProgram
    Keeper = 'Alice'; WorldName = "Alice's world"
}
Check "naming them again restored both" ($restore.ExitCode -eq 0) $restore.Output
$hRestored = Get-Content -Raw -LiteralPath $hProfilePath | ConvertFrom-Json
Check "a named flag still wins over what was stored" `
    ($hRestored.keeper -eq 'Alice' -and $hRestored.worldName -eq "Alice's world") `
    ("keeper='$($hRestored.keeper)' worldName='$($hRestored.worldName)'")

# WHAT AN UPGRADE RECORDS IS WHAT THE UNINSTALL CAN PUT BACK. The framework this
# package unpacked into this game folder on the FIRST install is still this
# install's after every run above it, so the uninstall takes it back out rather
# than leaving it in a game folder it promised to leave as it found it. The
# record is read again here, because it is the newest one that matters.
$hRecordAfter = Get-Content -Raw -LiteralPath $hRecordPath | ConvertFrom-Json
Check "the upgrade still owns the framework it put in this game folder" `
    ($hRecordAfter.bepInEx.installedByThisInstaller -eq $true)
Check "and still names its files, so the uninstall can take them" `
    (@($hRecordAfter.bepInEx.files).Count -gt 0)

$hUninstaller = Join-Path $hProgram 'Uninstall-BibitesMultiverse.ps1'
$uninstall = Invoke-Script $hUninstaller @{ DataRoot = $hData }
Check "the uninstall after every upgrade above succeeded" ($uninstall.ExitCode -eq 0) $uninstall.Output
Check "the mod framework went out of the game folder" `
    (-not (Test-Path -LiteralPath (Join-Path $hGame 'BepInEx\core\BepInEx.dll'))) $uninstall.Output
Check "the game itself is untouched" (Test-Path -LiteralPath (Join-Path $hGame 'The Bibites.exe'))
Check "the journal survived the uninstall" `
    ((Compare-Snapshot $hDataBefore (Get-TreeSnapshot (Join-Path $hData 'data'))).Count -eq 0)
Check "so did the logs" (Test-Path -LiteralPath $hLog)
Check "and so did this world's identity" (Test-Path -LiteralPath $hCredential)

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
